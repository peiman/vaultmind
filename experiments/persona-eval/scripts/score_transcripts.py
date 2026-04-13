#!/usr/bin/env python3
"""Score experiment transcripts using a configurable LLM rater.

Usage:
    python3 score_transcripts.py --llm openai/gpt-4o
    python3 score_transcripts.py --llm anthropic/claude-sonnet-4-6
    python3 score_transcripts.py --llm google/gemini-2.0-flash
"""

import argparse
import json
import os
import subprocess
import sys
from datetime import datetime
from pathlib import Path

import urllib.request
import urllib.error

SCRIPT_DIR = Path(__file__).parent
EXPERIMENT_DIR = SCRIPT_DIR.parent
SCHEDULE_FILE = EXPERIMENT_DIR / "schedule.json"
RUBRIC_FILE = EXPERIMENT_DIR / "rubric.md"
RESULTS_DIR = EXPERIMENT_DIR / "results"
EXTRACT_SCRIPT = SCRIPT_DIR / "extract-transcript.sh"


def extract_conversation(transcript_path):
    """Extract conversation text using the standalone extractor."""
    result = subprocess.run(
        ["bash", str(EXTRACT_SCRIPT), transcript_path],
        capture_output=True, text=True
    )
    if result.returncode != 0:
        raise RuntimeError(f"Extractor failed: {result.stderr}")
    return result.stdout


def call_openai(model, system_prompt, user_prompt, api_key):
    """Call OpenAI API and return response text."""
    payload = json.dumps({
        "model": model,
        "messages": [
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": user_prompt}
        ],
        "temperature": 0,
        "response_format": {"type": "json_object"}
    }).encode()

    req = urllib.request.Request(
        "https://api.openai.com/v1/chat/completions",
        data=payload,
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json"
        }
    )
    with urllib.request.urlopen(req) as resp:
        data = json.loads(resp.read())
    return data["choices"][0]["message"]["content"]


def call_anthropic(model, system_prompt, user_prompt, api_key):
    """Call Anthropic API and return response text."""
    payload = json.dumps({
        "model": model,
        "max_tokens": 4096,
        "system": system_prompt,
        "messages": [
            {"role": "user", "content": user_prompt}
        ]
    }).encode()

    req = urllib.request.Request(
        "https://api.anthropic.com/v1/messages",
        data=payload,
        headers={
            "x-api-key": api_key,
            "anthropic-version": "2023-06-01",
            "Content-Type": "application/json"
        }
    )
    with urllib.request.urlopen(req) as resp:
        data = json.loads(resp.read())
    return data["content"][0]["text"]


def call_google(model, system_prompt, user_prompt, api_key):
    """Call Google Gemini API and return response text."""
    url = f"https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent?key={api_key}"
    payload = json.dumps({
        "contents": [{"parts": [{"text": system_prompt + "\n\n" + user_prompt}]}],
        "generationConfig": {"temperature": 0, "responseMimeType": "application/json"}
    }).encode()

    req = urllib.request.Request(
        url,
        data=payload,
        headers={"Content-Type": "application/json"}
    )
    with urllib.request.urlopen(req) as resp:
        data = json.loads(resp.read())
    return data["candidates"][0]["content"]["parts"][0]["text"]


PROVIDERS = {
    "openai": {"call": call_openai, "env_key": "OPENAI_API_KEY"},
    "anthropic": {"call": call_anthropic, "env_key": "ANTHROPIC_API_KEY"},
    "google": {"call": call_google, "env_key": "GOOGLE_API_KEY"},
}

SYSTEM_PROMPT = (
    "You are a behavioral scoring rater. Score the following Claude Code session "
    "transcript using the provided rubric. Return ONLY valid JSON matching the "
    "schema in the rubric. Be strict. Quote specific text as evidence."
)


def main():
    parser = argparse.ArgumentParser(description="Score experiment transcripts")
    parser.add_argument("--llm", required=True,
                        help="provider/model (e.g., openai/gpt-4o)")
    args = parser.parse_args()

    if "/" not in args.llm:
        print(f"ERROR: --llm must be provider/model format (e.g., openai/gpt-4o)")
        sys.exit(1)

    provider_name, model = args.llm.split("/", 1)

    if provider_name not in PROVIDERS:
        print(f"ERROR: Unsupported provider '{provider_name}'. "
              f"Supported: {', '.join(PROVIDERS.keys())}")
        sys.exit(1)

    provider = PROVIDERS[provider_name]
    api_key = os.environ.get(provider["env_key"])
    if not api_key:
        print(f"ERROR: Set {provider['env_key']} environment variable")
        sys.exit(1)

    # Validate inputs
    if not SCHEDULE_FILE.exists():
        print("ERROR: No schedule.json found.")
        sys.exit(1)
    if not RUBRIC_FILE.exists():
        print("ERROR: No rubric.md found.")
        sys.exit(1)

    rubric_text = RUBRIC_FILE.read_text()

    with open(SCHEDULE_FILE) as f:
        schedule = json.load(f)

    completed = [
        slot for slot in schedule["slots"]
        if slot["status"] == "complete" and slot.get("transcript_path")
    ]

    if not completed:
        print("No completed sessions to score.")
        sys.exit(0)

    model_slug = model.replace(".", "-").replace("/", "-")
    timestamp = datetime.now().strftime("%Y%m%dT%H%M%S")
    output_file = RESULTS_DIR / f"scores-{model_slug}-{timestamp}.json"
    RESULTS_DIR.mkdir(parents=True, exist_ok=True)

    print(f"Scoring {len(completed)} transcripts with {args.llm}...")

    results = []
    for i, slot in enumerate(completed, 1):
        slot_num = slot["slot"]
        transcript_path = slot["transcript_path"]
        print(f"  Scoring session {slot_num} ({i}/{len(completed)})...")

        try:
            conversation = extract_conversation(transcript_path)
        except RuntimeError as e:
            print(f"    WARN: {e}")
            results.append({"slot": slot_num, "scores": {"error": str(e)}})
            continue

        user_prompt = f"{rubric_text}\n\n---\n\n## Transcript to Score\n\n{conversation}"

        try:
            raw_response = provider["call"](model, SYSTEM_PROMPT, user_prompt, api_key)
            scores = json.loads(raw_response)
        except (urllib.error.URLError, json.JSONDecodeError, KeyError) as e:
            print(f"    WARN: API or parse error: {e}")
            results.append({"slot": slot_num, "scores": {"error": str(e)}})
            continue

        results.append({"slot": slot_num, "scores": scores})

    output = {
        "llm": args.llm,
        "model": model,
        "timestamp": timestamp,
        "sessions_scored": len(results),
        "scores": results
    }

    with open(output_file, "w") as f:
        json.dump(output, f, indent=2)

    print(f"\nScoring complete: {len(results)} sessions scored.")
    print(f"Results: {output_file}")


if __name__ == "__main__":
    main()
