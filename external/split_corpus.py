#!/usr/bin/env python3
"""
按语义分割医患对话语料库文件
每个输出文件严格控制在 50MB 以下
采用固定行数 + 迭代调整
"""

import os
import csv
from pathlib import Path

DATA_DIR = Path("/Users/junjunyi/src-code/doctor-agent/external/medical_dialogue/data")
OUTPUT_DIR = Path("/Users/junjunyi/src-code/doctor-agent/external/medical_dialogue/data_split")

# 目标文件大小阈值 (50MB)
MAX_SIZE_MB = 48

def split_csv_by_size(input_file: Path):
    """按文件大小分割CSV文件"""
    print(f"\n处理: {input_file.name}")

    # 读取表头
    with open(input_file, 'r', encoding='utf-8', errors='replace') as f:
        reader = csv.reader(f)
        fieldnames = next(reader)

    # 读取所有数据
    rows = []
    with open(input_file, 'r', encoding='utf-8', errors='replace') as f:
        reader = csv.DictReader(f)
        for row in reader:
            rows.append(row)

    total_rows = len(rows)
    file_size_mb = input_file.stat().st_size / (1024 * 1024)
    print(f"  总行数: {total_rows:,}, 文件大小: {file_size_mb:.1f}MB")

    if file_size_mb <= MAX_SIZE_MB:
        print(f"  无需分割")
        return

    # 使用固定行数，保守估算
    # 假设每行约 500 bytes，48MB 约 100000 行
    # 为了安全，使用 50000 行/块
    rows_per_chunk = 50000

    print(f"  每块 {rows_per_chunk:,} 行")

    # 分割并写入
    base_name = input_file.stem
    i = 0
    start = 0
    while start < total_rows:
        end = min(start + rows_per_chunk, total_rows)
        chunk = rows[start:end]

        output_file = OUTPUT_DIR / f"{base_name}_part{i+1}.csv"

        with open(output_file, 'w', encoding='utf-8', newline='') as f:
            writer = csv.DictWriter(f, fieldnames=fieldnames)
            writer.writeheader()
            writer.writerows(chunk)

        file_size = output_file.stat().st_size / (1024 * 1024)

        # 如果超过50MB，减少行数重试
        if file_size > MAX_SIZE_MB and len(chunk) > 10000:
            # 减少行数
            new_rows_per_chunk = int(len(chunk) * (MAX_SIZE_MB / file_size) * 0.9)
            print(f"    part{i+1} 超限 ({file_size:.1f}MB)，调整为 {new_rows_per_chunk:,} 行")

            # 重新写入
            output_file.unlink()
            chunk = rows[start:start + new_rows_per_chunk]

            with open(output_file, 'w', encoding='utf-8', newline='') as f:
                writer = csv.DictWriter(f, fieldnames=fieldnames)
                writer.writeheader()
                writer.writerows(chunk)

            file_size = output_file.stat().st_size / (1024 * 1024)
            end = start + new_rows_per_chunk

        print(f"    part{i+1}: {len(chunk):,} 行, {file_size:.1f}MB")

        start = end
        i += 1

    print(f"  分割为 {i} 个文件")

def main():
    # 创建输出目录
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    # 获取所有CSV文件
    csv_files = list(DATA_DIR.glob("*.csv"))
    csv_files = [f for f in csv_files if "_part" not in f.name]

    print(f"找到 {len(csv_files)} 个CSV文件")

    for csv_file in sorted(csv_files):
        split_csv_by_size(csv_file)

    print(f"\n完成! 输出目录: {OUTPUT_DIR}")

    # 统计输出
    output_files = sorted(OUTPUT_DIR.glob("*.csv"))
    total_size = sum(f.stat().st_size for f in output_files) / (1024 * 1024)
    print(f"输出文件数: {len(output_files)}, 总大小: {total_size:.1f}MB")

    # 检查是否有超过50MB的文件
    over_limit = [f for f in output_files if f.stat().st_size > 50 * 1024 * 1024]
    if over_limit:
        print(f"\n⚠️ 警告: 以下文件仍超过50MB:")
        for f in over_limit:
            print(f"  {f.name}: {f.stat().st_size / (1024*1024):.1f}MB")
    else:
        print(f"\n✅ 所有文件都小于50MB")

if __name__ == "__main__":
    main()