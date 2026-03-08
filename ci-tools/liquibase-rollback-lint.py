#!/usr/bin/env python3
"""
liquibase-rollback-lint.py

Lints Liquibase changelog XML files in db/changelog/ for rollback safety.

Rules:
  - Rollback blocks may ONLY contain:
      dropTable, dropColumn, dropIndex, dropForeignKeyConstraint,
      dropUniqueConstraint, dropSequence, sql
  - Rollback blocks must NOT contain:
      modifyDataType, addColumn, createTable, addForeignKeyConstraint,
      renameColumn, renameTable
  - SQL rollback blocks scanned for VARCHAR shrink patterns — flagged as warning
  - Forward migrations using modifyDataType on VARCHAR — flagged as warning

Exit 0 = clean. Exit 1 = violations, PR blocked.

Usage:
  python3 liquibase-rollback-lint.py [path/to/db/changelog/]
"""

import re
import sys
import os
import glob
import xml.etree.ElementTree as ET

ALLOWED_ROLLBACK_TAGS = {
    "dropTable",
    "dropColumn",
    "dropIndex",
    "dropForeignKeyConstraint",
    "dropUniqueConstraint",
    "dropSequence",
    "sql",
}

FORBIDDEN_ROLLBACK_TAGS = {
    "modifyDataType",
    "addColumn",
    "createTable",
    "addForeignKeyConstraint",
    "renameColumn",
    "renameTable",
}

# Matches patterns like VARCHAR(255) → VARCHAR(100) in rollback SQL blocks.
VARCHAR_SHRINK_PATTERN = re.compile(
    r"varchar\s*\(\s*(\d+)\s*\)",
    re.IGNORECASE,
)

LIQUIBASE_NS = "http://www.liquibase.org/xml/ns/dbchangelog"


def local(tag: str) -> str:
    """Strip XML namespace prefix."""
    if tag.startswith("{"):
        return tag.split("}", 1)[1]
    return tag


def lint_file(path: str) -> tuple[list[str], list[str]]:
    """Returns (violations, warnings) for a single changelog file."""
    violations: list[str] = []
    warnings: list[str] = []

    try:
        tree = ET.parse(path)
    except ET.ParseError as exc:
        violations.append(f"{path}: XML parse error: {exc}")
        return violations, warnings

    root = tree.getroot()

    for changeset in root.iter():
        if local(changeset.tag) != "changeSet":
            continue

        changeset_id = changeset.get("id", "<unknown>")
        author = changeset.get("author", "<unknown>")
        ref = f"{path} changeSet id={changeset_id!r} author={author!r}"

        # Check forward changes for modifyDataType on VARCHAR (warning only).
        for child in changeset:
            tag = local(child.tag)
            if tag == "modifyDataType":
                new_type = child.get("newDataType", "")
                if "varchar" in new_type.lower():
                    warnings.append(
                        f"WARNING {ref}: forward modifyDataType to VARCHAR "
                        f"({new_type!r}) — ensure rollback handles potential data truncation"
                    )

        # Inspect rollback blocks.
        for rollback in changeset.iter():
            if local(rollback.tag) != "rollback":
                continue

            rollback_tags_found: set[str] = set()
            for rb_child in rollback:
                rb_tag = local(rb_child.tag)
                rollback_tags_found.add(rb_tag)

                if rb_tag in FORBIDDEN_ROLLBACK_TAGS:
                    violations.append(
                        f"VIOLATION {ref}: rollback contains forbidden tag <{rb_tag}>"
                    )
                elif rb_tag not in ALLOWED_ROLLBACK_TAGS:
                    violations.append(
                        f"VIOLATION {ref}: rollback contains unrecognised tag <{rb_tag}> "
                        f"(not in allowed list)"
                    )

            # Scan SQL rollback text for VARCHAR shrink patterns.
            rollback_sql = rollback.text or ""
            for sql_child in rollback.iter():
                if local(sql_child.tag) == "sql":
                    rollback_sql += sql_child.text or ""

            varchar_sizes = VARCHAR_SHRINK_PATTERN.findall(rollback_sql)
            if len(varchar_sizes) >= 2:
                sizes = [int(s) for s in varchar_sizes]
                # Heuristic: if sizes decrease this looks like a shrink.
                if sizes != sorted(sizes, reverse=True):
                    warnings.append(
                        f"WARNING {ref}: rollback SQL may shrink VARCHAR "
                        f"(found sizes {sizes}) — verify data truncation risk"
                    )

    return violations, warnings


def main() -> int:
    search_path = sys.argv[1] if len(sys.argv) > 1 else "db/changelog"

    pattern = os.path.join(search_path, "**", "*.xml")
    files = glob.glob(pattern, recursive=True)

    if not files:
        print(f"No changelog XML files found under {search_path!r}")
        return 0

    all_violations: list[str] = []
    all_warnings: list[str] = []

    for f in sorted(files):
        v, w = lint_file(f)
        all_violations.extend(v)
        all_warnings.extend(w)

    for w in all_warnings:
        print(w)

    for v in all_violations:
        print(v)

    if all_violations:
        print(f"\n{len(all_violations)} violation(s) found — PR blocked.")
        return 1

    print(f"Linted {len(files)} file(s). No violations found.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
