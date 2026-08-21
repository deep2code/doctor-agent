#!/usr/bin/env python3
"""
Helper script to download LMKG from Google Drive.
Run this after placing the downloaded files in this directory,
or use it to verify the download.
"""
import os
import sys

DEST_DIR = os.path.dirname(os.path.abspath(__file__))

# Expected files based on the paper description
# (exact filenames will be confirmed after first manual download)
EXPECTED_PATTERNS = [
    "*.csv", "*.tsv", "*.nt", "*.n3", "*.ttl", "*.json",
    "*.zip", "*.tar.gz", "*.txt",
]

def check_existing_files():
    """Check what files already exist in the directory."""
    files = [f for f in os.listdir(DEST_DIR) if os.path.isfile(os.path.join(DEST_DIR, f))]
    if files:
        print("Existing files in directory:")
        for f in sorted(files):
            size = os.path.getsize(os.path.join(DEST_DIR, f))
            print(f"  {f} ({size:,} bytes)")
    else:
        print("No data files found. Please download from Google Drive:")
        print("  https://drive.google.com/drive/folders/13oDTHUnkgaN__pw_XKGEotmu97odmUpF")
    return files


if __name__ == "__main__":
    print("LMKG Dataset Directory")
    print("=" * 50)
    files = check_existing_files()

    if not files:
        print("\nTo download:")
        print("1. Open in browser: https://drive.google.com/drive/folders/13oDTHUnkgaN__pw_XKGEotmu97odmUpF")
        print("2. Download all files")
        print(f"3. Place them in: {DEST_DIR}")
        print("4. Run this script again to verify")
    else:
        print(f"\nFound {len(files)} files. Ready for processing.")
