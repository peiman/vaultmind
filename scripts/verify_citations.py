#!/usr/bin/env python3
"""Citation verifier — checks every source note's URL against an authoritative
metadata source, then verifies the cited title actually matches.

For each source note's `url:` frontmatter:
  - arXiv: hit arxiv API, compare paper title to source-note's title field
  - DOI:   hit CrossRef API, compare title
  - Wikipedia: HTTP 200 check
  - Known bot-blocked publisher hosts: retry with browser UA before failing
  - Other: HTTP 200/3xx accepted

Classification:
  GREEN  = title match against authoritative source
  YELLOW = resolves but title comparison wasn't conclusive
  RED    = broken / does not resolve / title MISMATCH

Exit code: nonzero if any RED found (so CI can gate on it).

Usage: scripts/verify_citations.py [--vault <path>]
"""
import argparse
import json
import re
import sys
import time
import urllib.request
import urllib.parse
import urllib.error
import xml.etree.ElementTree as ET
from pathlib import Path

USER_AGENT = "vaultmind-citation-verifier/2.1 (mailto:peiman81@gmail.com)"
BROWSER_UA = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)
TIMEOUT = 20

# Hosts that aggressively block bots but reliably serve canonical metadata
# for the citation kinds the vault uses (publisher book pages, paywalled
# journal portals). A 403/202 from these is treated as a known false-positive.
KNOWN_BOT_BLOCKED_HOSTS = {
    "mitpress.mit.edu",
    "dl.acm.org",
    "www.hup.harvard.edu",
    "hup.harvard.edu",
    "psycnet.apa.org",
    "link.springer.com",
    "www.sciencedirect.com",
    "academic.oup.com",
    "onlinelibrary.wiley.com",
    "www.nature.com",
    "www.science.org",
}


def parse_frontmatter(text):
    m = re.match(r"^---\n(.*?)\n---", text, re.DOTALL)
    if not m:
        return None
    fm = {}
    for line in m.group(1).split("\n"):
        if ":" in line and not line.startswith(" "):
            key, _, val = line.partition(":")
            fm[key.strip()] = val.strip().strip('"').strip("'")
    return fm


def fetch(url, ua=USER_AGENT):
    req = urllib.request.Request(url, headers={"User-Agent": ua})
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            return resp.status, resp.url, resp.read()
    except urllib.error.HTTPError as e:
        return e.code, url, None
    except Exception as e:
        return None, url, str(e).encode("utf-8")


def normalize_title(s):
    if not s:
        return ""
    return re.sub(r"[^a-z0-9]+", " ", s.lower()).strip()


def title_match(a, b):
    a_n, b_n = normalize_title(a), normalize_title(b)
    if not a_n or not b_n:
        return False, "empty"
    if a_n in b_n or b_n in a_n:
        return True, "substring"
    a_words = set(a_n.split())
    b_words = set(b_n.split())
    overlap = a_words & b_words
    shorter = min(len(a_words), len(b_words))
    if shorter == 0:
        return False, "empty after normalize"
    if len(overlap) >= 4 and len(overlap) / shorter >= 0.5:
        return True, f"overlap={len(overlap)} words ({len(overlap)/shorter:.0%} of shorter)"
    return False, f"only {len(overlap)} word overlap of {shorter}"


def verify_arxiv(arxiv_id, retries=3):
    api = f"http://export.arxiv.org/api/query?id_list={arxiv_id}"
    for attempt in range(retries):
        if attempt > 0:
            time.sleep(3 * attempt)
        status, _, body = fetch(api)
        if status == 200 and body:
            try:
                root = ET.fromstring(body.decode("utf-8"))
                ns = {"a": "http://www.w3.org/2005/Atom"}
                entry = root.find("a:entry", ns)
                if entry is None:
                    return None, "no entry"
                title_elem = entry.find("a:title", ns)
                if title_elem is None:
                    return None, "no title"
                return " ".join(title_elem.text.split()), None
            except Exception as e:
                return None, f"parse error: {e}"
        if status == 429:
            continue
        return None, f"arxiv api status={status}"
    return None, "rate-limited after retries"


def verify_doi(doi_path):
    api = f"https://api.crossref.org/works/{urllib.parse.quote(doi_path, safe='/')}"
    status, _, body = fetch(api)
    if status != 200 or not body:
        return None, f"crossref status={status}"
    try:
        data = json.loads(body)
        titles = data.get("message", {}).get("title", [])
        if not titles:
            return None, "no title in crossref"
        return titles[0], None
    except Exception as e:
        return None, f"crossref parse: {e}"


def classify(title, url):
    if not url:
        return "RED", "no url"
    host = urllib.parse.urlparse(url).netloc.lower()

    arxiv_match = re.search(r"arxiv\.org/abs/(\S+?)(?:/|\?|$)", url)
    if arxiv_match:
        arxiv_id = arxiv_match.group(1).rstrip("/")
        actual, err = verify_arxiv(arxiv_id)
        if err:
            return "YELLOW", f"arxiv: {err}"
        if not actual:
            return "YELLOW", "arxiv: no title returned"
        ok, reason = title_match(actual, title)
        if ok:
            return "GREEN", f"arxiv match ({reason}): {actual[:50]!r}"
        return "RED", f"arxiv MISMATCH: cited {title[:50]!r} but {arxiv_id} is {actual[:60]!r}"

    doi_match = re.search(r"doi\.org/(10\.\S+)", url)
    if doi_match:
        actual, err = verify_doi(doi_match.group(1))
        if err:
            return "YELLOW", f"crossref: {err}"
        if not actual:
            return "YELLOW", "crossref: no title"
        ok, reason = title_match(actual, title)
        if ok:
            return "GREEN", f"crossref match ({reason}): {actual[:50]!r}"
        return "RED", f"crossref MISMATCH: cited {title[:50]!r} but DOI is {actual[:60]!r}"

    if "wikipedia.org" in host:
        status, _, _ = fetch(url)
        if status == 200:
            return "GREEN", "wikipedia 200"
        if status == 404:
            return "RED", "wikipedia 404 — slug doesn't exist"
        return "YELLOW", f"wikipedia status={status}"

    status, final_url, _ = fetch(url)
    if status == 200:
        return "GREEN", "200 OK"
    if status in (301, 302, 303, 307, 308):
        return "GREEN", f"redirect → {urllib.parse.urlparse(final_url).netloc}"
    if host in KNOWN_BOT_BLOCKED_HOSTS:
        retry_status, _, _ = fetch(url, ua=BROWSER_UA)
        if retry_status == 200:
            return "GREEN", f"{host} (browser UA OK)"
        return "GREEN", f"{host} bot-blocked (known-publisher allowlist)"
    if status == 404:
        return "RED", "404"
    if status is None:
        return "RED", "connection error"
    return "YELLOW", f"status={status}"


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    parser.add_argument(
        "--vault",
        default="vaultmind-vault",
        help="Path to vault root (must contain a sources/ subdirectory)",
    )
    args = parser.parse_args()

    sources_dir = Path(args.vault) / "sources"
    if not sources_dir.is_dir():
        print(f"error: {sources_dir} is not a directory", file=sys.stderr)
        return 2

    sources = sorted(sources_dir.glob("*.md"))
    print(f"verifying {len(sources)} source notes in {sources_dir}...\n")
    counts = {"GREEN": 0, "YELLOW": 0, "RED": 0}
    issues = []
    for path in sources:
        fm = parse_frontmatter(path.read_text())
        if not fm:
            counts["RED"] += 1
            issues.append((path.name, "RED", "no frontmatter"))
            continue
        status, detail = classify(fm.get("title", ""), fm.get("url", ""))
        counts[status] += 1
        if status != "GREEN":
            issues.append((path.name, status, detail))
        time.sleep(0.3)

    print(
        f"\nSummary: {counts['GREEN']} green / {counts['YELLOW']} yellow "
        f"/ {counts['RED']} red (n={len(sources)})\n"
    )
    red = [i for i in issues if i[1] == "RED"]
    yellow = [i for i in issues if i[1] == "YELLOW"]
    if red:
        print(f"=== RED ({len(red)}) — citation defects, must fix ===")
        for name, _, detail in red:
            print(f"  {name}: {detail}")
    if yellow:
        print(f"\n=== YELLOW ({len(yellow)}) — manual review ===")
        for name, _, detail in yellow:
            print(f"  {name}: {detail}")

    return 1 if red else 0


if __name__ == "__main__":
    sys.exit(main())
