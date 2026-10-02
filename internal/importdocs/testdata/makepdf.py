"""Writes the PDF fixtures for the import tests: minimal, valid PDFs with a
correct xref table. Re-run from this directory to regenerate them."""


def pdf(pages, title=None):
    objs = ["<< /Type /Catalog /Pages 2 0 R >>"]
    kids = " ".join(f"{3 + 2 * i} 0 R" for i in range(len(pages)))
    objs.append(f"<< /Type /Pages /Kids [{kids}] /Count {len(pages)} >>")
    font = 3 + 2 * len(pages)
    for i, lines in enumerate(pages):
        content = 4 + 2 * i
        objs.append(f"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "
                    f"/Resources << /Font << /F1 {font} 0 R >> >> /Contents {content} 0 R >>")
        ops = "BT /F1 12 Tf 72 720 Td 14 TL " + " ".join(
            "(" + l.replace("\\", "\\\\").replace("(", "\\(").replace(")", "\\)") + ") '" for l in lines) + " ET"
        objs.append(f"<< /Length {len(ops)} >>\nstream\n{ops}\nendstream")
    objs.append("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
    info = None
    if title is not None:
        objs.append(f"<< /Title ({title}) >>")
        info = len(objs)
    out = b"%PDF-1.4\n"
    offsets = []
    for n, o in enumerate(objs, 1):
        offsets.append(len(out))
        out += f"{n} 0 obj\n{o}\nendobj\n".encode("latin-1")
    xref = len(out)
    out += f"xref\n0 {len(objs) + 1}\n0000000000 65535 f \n".encode()
    for off in offsets:
        out += f"{off:010d} 00000 n \n".encode()
    trailer = f"<< /Size {len(objs) + 1} /Root 1 0 R"
    if info:
        trailer += f" /Info {info} 0 R"
    out += f"trailer\n{trailer} >>\nstartxref\n{xref}\n%%EOF\n".encode()
    return out


with open("paper.pdf", "wb") as f:
    f.write(pdf([
        ["Retrieval for Agent Memory", "",
         "Agents forget between sessions. We study embed-",
         "ding long notes so that recall survives the boundary."],
        ["Results", "", "Sparse retrieval beats dense retrieval on long documents."],
    ], title="Retrieval for Agent Memory: A Study"))

with open("placeholder-title.pdf", "wb") as f:
    f.write(pdf([["Spreading Activation in Practice", "", "Activation spreads along edges to related notes."]],
                title="Microsoft Word - draft3.docx"))

with open("no-text.pdf", "wb") as f:
    f.write(pdf([[]]))
