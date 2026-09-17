#!/usr/bin/env python3
"""Parse the WHO Model List of Essential Medicines 24th list (2025) from the
PDF-extracted text (external/eml/eml-24th.txt) into structured JSON.

Output schema:
  {"source": "WHO Model List of Essential Medicines, 24th list (2025)",
   "url": "https://www.who.int/publications/i/item/B09474",
   "updated": "...",
   "entries": [
     {"name": "cefotaxime", "section": "6.2.1 Beta lactam medicines",
      "list": "core"|"complementary", "forms": ["Powder for injection: ..."],
      "indications": [{"choice": "first"|"second", "text": "Acute bacterial meningitis"}],
      "note": "...", "children": bool, "square_box": bool,
      "therapeutic_alternatives": ["..."]}
   ]}

Idempotent: regenerates external/eml/eml_24_structured.json from the source text.
"""
import json
import re
import sys
from pathlib import Path

SRC = Path(__file__).parent / "eml" / "eml-24th.txt"
OUT = Path(__file__).parent / "eml" / "eml_24_structured.json"

# Dosage-form keywords that mark the boundary between drug name and rest.
DOSAGE = re.compile(
    r"\s+(?=(?:Inhalation|Injection|Tablet|Capsule|Oral|Suppository|Powder|"
    r"Solution|Granules|Dental|Topical|Transdermal|Concentrate|Rectal|Solid|"
    r"Syrup|Spray|Oily|Eye|Lotion|Gel|Patch|Ampoule|Emulsion|Suspension|"
    r"Enema|Pessary|Cream|Ointment|Drops|Sublingual|Inhaler|Infusion|Medicated))"
)
SECTION_RE = re.compile(r"^\d+(\.\d+)*\.?\s+[A-Z]")
PAGE_HEADER = "WHO Model List of Essential Medicines"


def clean_name(raw: str) -> tuple[str, bool, bool]:
    """Return (name, children, square_box) after stripping markers."""
    s = raw.strip()
    square = bool(re.search(r"[\ue000-\uf8ff□▢]", s))
    s = re.sub(r"[\ue000-\uf8ff□▢]", "", s)
    children = bool(re.search(r"\[c\]", s))
    s = s.replace("[c]", "").strip()
    # trailing footnote letters, e.g. "meropenem* a" / "ceftriaxone*a"
    if len(s) > 3 and re.search(r"\s[a-z]$", s):
        s = re.sub(r"\s[a-z]$", "", s)
    s = re.sub(r"\*[a-z]?$", "", s).strip()
    s = s.rstrip("†‡").strip()
    # trailing footnote letter, e.g. "ibuprofen a" -> "ibuprofen"
    if len(s) > 3 and re.search(r"\s[a-z]$", s):
        s = re.sub(r"\s[a-z]$", "", s)
    s = re.sub(r"\s+", " ", s).strip()
    return s, children, square


def split_name(line: str) -> tuple[str, str, bool, bool]:
    """Split a top-level line into (name, rest, children, square_box)."""
    m = DOSAGE.search(line)
    if m:
        name_part = line[: m.start()]
        rest = line[m.start() :].strip()
    else:
        name_part = line
        rest = ""
    name, children, square = clean_name(name_part)
    return name, rest, children, square


# Top-level dosage-form lines (PDF extraction lost the indentation of
# continuation lines). These belong to the current entry's forms.
FORM_START = re.compile(
    r"^(?:Oral liquid|Oral|Injection|Tablet|Capsule|Powder|Solution|Suppository|"
    r"Granules|Dental|Topical|Transdermal|Concentrate|Rectal|Solid|Syrup|Spray|"
    r"Oily|Eye|Lotion|Gel|Patch|Ampoule|Emulsion|Suspension|Enema|Pessary|Cream|"
    r"Ointment|Drops|Sublingual|Inhaler|Inhalation|Infusion|Medicated|Intravenous|"
    r"Intramuscular|Intrathecal|Intraperitoneal|Intravesical|Implant|Insert|Nebulizer|Unit)"
)


# Fragments produced by PDF extraction that must not become entries.
DROPLIST = {
    "(methylene blue)", "(Prussian blue)", "risk of birth defects and",
    "exposed to valproic acid (sodium", "(mild to moderate)", "(severe)",
    "infections (mild to moderate)", "(J01CF Beta-lactamase resistant",
    "pharyngitis in children (EMLc only)", "infections and high-risk febrile",
    "neutropenia only. Meropenem is", "the preferred choice for acute",
    "anaesthetics) excluding cocaine and combinations",
    "ampoules for use in nebulizers;", "intended to be swallowed whole;",
    "potassium ferric hexacyano-ferrate(II) -2H2O",
}


def main() -> int:
    text = SRC.read_text(encoding="utf-8")
    lines = text.splitlines()

    # Slice the body: from the first section heading to the Index.
    start = next(i for i, l in enumerate(lines) if l.strip().startswith("1. ANAESTHETICS"))
    end = next(i for i, l in enumerate(lines) if l.strip() == "Index")
    body = lines[start:end]

    entries = []
    current = None
    section = ""
    list_type = "core"
    choice = None  # "first" | "second" | "both"
    ta_mode = False

    for raw in body:
        line = raw.rstrip()
        s = line.strip()
        if not s:
            continue
        # page headers
        if PAGE_HEADER in line and ("page" in line or "List (2025)" in line):
            continue
        # section headings (also resets the list type: each section starts
        # with the core list unless a Complementary List marker follows)
        if SECTION_RE.match(s):
            section = s
            list_type = "core"
            ta_mode = False
            continue
        if s.startswith("Complementary List"):
            list_type = "complementary"
            continue
        # choice markers
        if s in ("FIRST CHOICE", "SECOND CHOICE", "FIRST CHOICE SECOND CHOICE"):
            if s == "FIRST CHOICE SECOND CHOICE":
                choice = "both"
            else:
                choice = "first" if "FIRST" in s else "second"
            continue
        # indications / therapeutic alternatives bullets
        if re.match(r"^[−\-\u2022]\s+", s):
            bullet = re.sub(r"^[−\-\u2022]\s+", "", s)
            if ta_mode and current:
                current["therapeutic_alternatives"].append(bullet)
            elif current and choice:
                current["indications"].append({"choice": choice, "text": bullet})
            continue
        if s.startswith("Therapeutic alternatives"):
            ta_mode = True
            if current:
                current["therapeutic_alternatives"] = []
            continue
        # indented continuation lines -> forms/notes of the current entry
        if line != line.lstrip():
            if current is None:
                continue
            if s.startswith("*"):
                current["note"] = (current["note"] + " " + s).strip()
            else:
                current["forms"].append(s)
            ta_mode = False
            continue

        # footnote lines ("a Not in children ...") and top-level star notes
        if re.match(r"^[a-z]\s+[A-Z]", s) or re.match(r"^[a-z]\s*[><≥≤]", s) or s.startswith("*"):
            if current and s.startswith("*"):
                current["note"] = (current["note"] + " " + s).strip()
            ta_mode = False
            continue
        # top-level dosage-form continuation of the current entry
        if current and FORM_START.match(s):
            current["forms"].append(s)
            ta_mode = False
            continue
        # top-level line -> new drug entry (must be a lowercase INN-style name)
        name, rest, children, square = split_name(line)
        if (
            not name
            or name in DROPLIST
            or name[0].isupper()
            or name[0].isdigit()
            or "..." in name
            or name.endswith(".")
            # prose fragments (footnote/paragraph spillover) are long and
            # carry no dosage form or indication of their own
            or (len(name.split()) >= 6 and not rest)
        ):
            ta_mode = False
            continue
        if current is not None:
            entries.append(current)
        current = {
            "name": name,
            "section": section,
            "list": list_type,
            "forms": [rest] if rest else [],
            "indications": [],
            "note": "",
            "children": children,
            "square_box": square,
            "therapeutic_alternatives": [],
        }
        ta_mode = False
        choice = None

    if current is not None:
        entries.append(current)

    # dedupe by name (same drug may appear in multiple sections)
    def uniq(xs):
        return list(dict.fromkeys(xs))

    seen = {}
    for e in entries:
        key = e["name"]
        if key in seen:
            seen[key]["forms"] = uniq(seen[key]["forms"] + e["forms"])
            ind = {f"{i['choice']}|{i['text']}": i for i in seen[key]["indications"] + e["indications"]}
            seen[key]["indications"] = list(ind.values())
            seen[key]["note"] = " ".join(dict.fromkeys((seen[key]["note"] + " " + e["note"]).split()))
            seen[key]["children"] = seen[key]["children"] or e["children"]
            seen[key]["square_box"] = seen[key]["square_box"] or e["square_box"]
            seen[key]["therapeutic_alternatives"] = uniq(
                seen[key]["therapeutic_alternatives"] + e["therapeutic_alternatives"]
            )
            # a drug listed in the core list anywhere is core overall
            if e["list"] == "core":
                seen[key]["list"] = "core"
        else:
            seen[key] = e
    deduped = list(seen.values())

    out = {
        "source": "WHO Model List of Essential Medicines, 24th list (2025)",
        "url": "https://www.who.int/publications/i/item/B09474",
        "updated": "2026-08-09",
        "entries": deduped,
    }
    OUT.write_text(json.dumps(out, ensure_ascii=False, indent=1), encoding="utf-8")

    # Knowledge-base format (internal/knowledge/data/who_eml.json): entries get
    # an empty name_zh slot for later LLM translation.
    kb = {
        "source": "WHO Model List of Essential Medicines, 24th list (2025)",
        "url": "https://www.who.int/publications/i/item/B09474",
        "updated": "2026-08-09",
        "entries": [
            {
                "name": e["name"],
                "name_zh": "",
                "section": e["section"],
                "list": e["list"],
                "forms": e["forms"],
                "indications": e["indications"],
                "note": e["note"],
                "children": e["children"],
                "square_box": e["square_box"],
                "therapeutic_alternatives": e["therapeutic_alternatives"],
            }
            for e in deduped
        ],
    }
    KB_OUT = Path(__file__).parent.parent / "internal" / "knowledge" / "data" / "who_eml.json"
    KB_OUT.write_text(json.dumps(kb, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"entries: {len(deduped)} (raw {len(entries)}) -> {OUT}")
    print(f"knowledge json: {len(kb['entries'])} entries -> {KB_OUT}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
