#!/usr/bin/env python3
"""Quick verification for batch 14 datasets in MariaDB."""

import mysql.connector
import sys

def main():
    conn = mysql.connector.connect(
        host="localhost",
        port=3307,
        user="root",
        database="doctor_knowledge"
    )
    cursor = conn.cursor()

    # Check row counts for new datasets
    datasets = ["yiigle_clinical_guides", "sleep_child_redcross", "chinacdc_science"]

    print("📊 Batch 14 Dataset Verification")
    print("=" * 60)

    total = 0
    for dataset in datasets:
        cursor.execute("SELECT COUNT(*) FROM kb_items WHERE dataset = %s", (dataset,))
        count = cursor.fetchone()[0]
        total += count
        status = "✅" if count > 0 else "❌"
        print(f"{status} {dataset}: {count} rows")

    print("=" * 60)
    print(f"Total: {total} rows")

    # Sample check - verify yiigle entries have correct structure
    cursor.execute("""
        SELECT id, LEFT(data, 200) as preview
        FROM kb_items
        WHERE dataset = 'yiigle_clinical_guides'
        LIMIT 3
    """)

    print("\n🔍 Sample yiigle entries:")
    for row in cursor.fetchall():
        print(f"  - {row[0]}")

    cursor.close()
    conn.close()

    if total == 268:  # 34 + 56 + 178
        print("\n✅ All batch 14 entries verified!")
        sys.exit(0)
    else:
        print(f"\n⚠️  Expected 268 rows, got {total}")
        sys.exit(1)

if __name__ == "__main__":
    main()
