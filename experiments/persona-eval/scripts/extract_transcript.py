#!/usr/bin/env python3
"""Extract readable USER/ASSISTANT turns from a Claude Code JSONL transcript."""
import json
import sys


def extract(transcript_path):
    turns = []
    with open(transcript_path) as f:
        for line in f:
            try:
                d = json.loads(line)
            except json.JSONDecodeError:
                continue

            t = d.get("type", "")

            # User messages — skip system-generated ones
            if t == "user" and isinstance(d.get("message"), dict):
                content = d["message"].get("content", "")
                if isinstance(content, str) and not content.startswith("<"):
                    turns.append(f"USER: {content}")
                elif isinstance(content, list):
                    # User follow-ups often have list content with text blocks
                    text_parts = []
                    for block in content:
                        if isinstance(block, dict) and block.get("type") == "text":
                            text = block.get("text", "")
                            if not text.startswith("<"):
                                text_parts.append(text)
                    if text_parts:
                        turns.append(f"USER: {chr(10).join(text_parts)}")

            # Assistant text blocks
            elif t == "assistant":
                msg = d.get("message", {})
                if isinstance(msg, dict):
                    content = msg.get("content", [])
                    if isinstance(content, list):
                        text_parts = []
                        for block in content:
                            if isinstance(block, dict) and block.get("type") == "text":
                                text_parts.append(block["text"])
                        if text_parts:
                            combined = "\n".join(text_parts)
                            turns.append(f"ASSISTANT: {combined}")

    # Deduplicate consecutive identical turns (streaming artifacts)
    deduped = []
    for turn in turns:
        if not deduped or turn != deduped[-1]:
            deduped.append(turn)

    return "\n\n".join(deduped)


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: extract_transcript.py <path-to-jsonl>", file=sys.stderr)
        sys.exit(1)
    print(extract(sys.argv[1]))
