#!/usr/bin/env python3
"""
批量下载中华医学会临床实践指南全文（中文）。

来源：
1. 中华医学会官网 https://www.cma.org.cn/ 指南库
2. 中国循环杂志 http://www.chinacirculation.org/ 指南共识栏目
3. 中华系列期刊 PDF 直链

策略：
- 先抓索引页，提取指南标题+URL
- 正文页用 browser-use 真浏览器绕过 JS 渲染
- PDF 附件直接 wget/curl
- 幂等：已下载的跳过
"""
import os
import sys
import json
import time
import hashlib
from pathlib import Path
from urllib.parse import urljoin, urlparse

# 配置
OUTPUT_DIR = Path(__file__).parent / "cma_guidelines"
RAW_DIR = OUTPUT_DIR / "raw"
MANIFEST_FILE = OUTPUT_DIR / "MANIFEST.txt"
MAX_CONCURRENT = 3  # 并发数
REQUEST_DELAY = 2   # 请求间隔秒

RAW_DIR.mkdir(parents=True, exist_ok=True)


def load_manifest():
    """加载已有清单，支持断点续跑。"""
    if MANIFEST_FILE.exists():
        with open(MANIFEST_FILE, 'r', encoding='utf-8') as f:
            return [line.strip() for line in f if line.strip()]
    return []


def save_manifest(entries):
    """保存清单。"""
    with open(MANIFEST_FILE, 'w', encoding='utf-8') as f:
        for entry in entries:
            f.write(entry + '\n')


def sha256_file(filepath):
    """计算文件SHA256。"""
    h = hashlib.sha256()
    with open(filepath, 'rb') as f:
        for chunk in iter(lambda: f.read(8192), b''):
            h.update(chunk)
    return h.hexdigest()


def fetch_page_html(url, timeout=30):
    """
    抓取页面HTML。优先尝试requests，失败则提示用browser-use。
    返回 (html_content, success)。
    """
    import subprocess
    try:
        # 先试 curl
        result = subprocess.run(
            ['curl', '-L', '-A', 'Mozilla/5.0', '--max-time', str(timeout), url],
            capture_output=True, text=True, timeout=timeout+5
        )
        if result.returncode == 0 and len(result.stdout) > 1000:
            return result.stdout, True
    except Exception:
        pass

    # curl 失败，可能是JS渲染或WAF
    print(f"  [WARN] curl 获取失败，可能需要 browser-use: {url}")
    return None, False


def extract_pdf_links(html, base_url):
    """从HTML中提取PDF链接。"""
    import re
    pdf_urls = []
    # 匹配 .pdf 链接
    pattern = r'href=["\']([^"\']+\.pdf[^"\']*)["\']'
    for match in re.finditer(pattern, html, re.IGNORECASE):
        pdf_url = match.group(1)
        if not pdf_url.startswith(('http://', 'https://')):
            pdf_url = urljoin(base_url, pdf_url)
        pdf_urls.append(pdf_url)
    return list(set(pdf_urls))


def download_pdf(pdf_url, output_dir, title_hint=""):
    """下载单个PDF，返回本地文件路径或None。"""
    import subprocess
    filename = pdf_url.split('/')[-1].split('?')[0]
    if not filename.endswith('.pdf'):
        filename = f"{title_hint or 'guideline'}_{hashlib.md5(pdf_url.encode()).hexdigest()[:8]}.pdf"

    filepath = output_dir / filename
    if filepath.exists() and filepath.stat().st_size > 1024:
        print(f"  [SKIP] 已存在: {filepath.name} ({filepath.stat().st_size/1024:.1f}KB)")
        return str(filepath)

    try:
        result = subprocess.run(
            ['curl', '-L', '-A', 'Mozilla/5.0', '--max-time', '60', '-o', str(filepath), pdf_url],
            capture_output=True, text=True, timeout=65
        )
        if result.returncode == 0 and filepath.stat().st_size > 1024:
            size_kb = filepath.stat().st_size / 1024
            print(f"  [OK] {filepath.name} ({size_kb:.1f}KB)")
            return str(filepath)
        else:
            filepath.unlink(missing_ok=True)
            print(f"  [FAIL] 下载失败: {pdf_url}")
            return None
    except Exception as e:
        filepath.unlink(missing_ok=True)
        print(f"  [ERROR] {e}")
        return None


def main():
    print("=" * 60)
    print("中华医学会指南批量下载器")
    print("=" * 60)

    existing = load_manifest()
    print(f"清单中已有 {len(existing)} 条记录")

    # 目标URL列表（人工维护，逐步扩充）
    target_urls = [
        # 中华医学会官网 - 临床实践指南
        "https://www.cma.org.cn/art/2020/12/24/art_12345.html",  # 示例，需替换为真实索引页
        # 中国循环杂志 - 指南共识
        "http://www.chinacirculation.org/",
        # 中华心血管病杂志
        "http://www.cjc.org.cn/",
        # 中华内科杂志
        "http://www.zhcnx.com/",
    ]

    # 注意：上述URL是占位符，实际需要从搜索引擎或人工收集真实索引页
    # 这里演示框架逻辑

    new_entries = []
    total_downloaded = 0

    for idx, url in enumerate(target_urls, 1):
        print(f"\n[{idx}/{len(target_urls)}] 处理: {url}")

        html, success = fetch_page_html(url)
        if not success:
            print(f"  [SKIP] 无法获取页面内容")
            continue

        pdf_urls = extract_pdf_links(html, url)
        print(f"  发现 {len(pdf_urls)} 个PDF链接")

        for pdf_url in pdf_urls[:20]:  # 限制每页最多20个PDF
            if pdf_url in existing:
                print(f"  [SKIP] 已下载: {pdf_url[:60]}...")
                continue

            filepath = download_pdf(pdf_url, RAW_DIR, title_hint=f"guideline_{total_downloaded}")
            if filepath:
                entry = f"{pdf_url}\t{filepath}\t{time.strftime('%Y-%m-%d')}"
                new_entries.append(entry)
                existing.append(pdf_url)
                total_downloaded += 1
                save_manifest(existing)

            time.sleep(REQUEST_DELAY)

    print(f"\n{'='*60}")
    print(f"本次新增 {total_downloaded} 个PDF")
    print(f"累计 {len(existing)} 条记录")
    print(f"输出目录: {RAW_DIR}")
    print(f"清单文件: {MANIFEST_FILE}")


if __name__ == '__main__':
    main()
