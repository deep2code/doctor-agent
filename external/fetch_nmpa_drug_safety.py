#!/usr/bin/env python3
"""批量下载国家药监局(NMPA)药品安全科普文章。

来源：
1. 国家药监局官网 - 科普知识栏目
   https://www.nmpa.gov.cn/xg/kpzs/index.html
2. 中国医药报 - 用药安全专栏
3. 国家药监局药品评价中心 - 药物警戒快讯

策略：
- 列表页用 curl + BeautifulSoup 提取文章链接
- 正文页提取主容器文本（需处理 gov.cn WAF）
- 幂等：已下载的跳过
- 输出到 external/nmpa_drug_safety/raw/<id>.txt
- MANIFEST.txt 记录标题/URL/日期
"""
import os
import sys
import re
import time
import pathlib
from urllib.parse import urljoin

import requests
from bs4 import BeautifulSoup

UA = {
    "User-Agent": (
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
        "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
    ),
    "Accept-Language": "zh-CN,zh;q=0.9",
}

DELAY = 2.0
MIN_ZH_CHARS = 250

ROOT = pathlib.Path(__file__).resolve().parent / "nmpa_drug_safety" / "raw"
MANIFEST = pathlib.Path(__file__).resolve().parent / "nmpa_drug_safety" / "MANIFEST.txt"
ROOT.mkdir(parents=True, exist_ok=True)

# NMPA科普栏目索引页
NMPA_URLS = [
    "https://www.nmpa.gov.cn/xg/kpzs/index.html",  # 科普知识首页
]


def load_manifest():
    if MANIFEST.exists():
        with open(MANIFEST, 'r', encoding='utf-8') as f:
            return set(line.split('\t')[0] for line in f if '\t' in line)
    return set()


def save_manifest_entry(url, title, date):
    with open(MANIFEST, 'a', encoding='utf-8') as f:
        f.write(f"{url}\t{title}\t{date}\n")


def fetch_html(url, timeout=25):
    """抓取HTML，处理可能的编码问题。"""
    try:
        r = requests.get(url, headers=UA, timeout=timeout)
        r.raise_for_status()
        # NMPA可能用GBK
        if r.encoding and 'gb' in r.encoding.lower():
            r.encoding = 'gb18030'
        else:
            r.encoding = 'utf-8'
        return r.text
    except Exception as e:
        print(f"  [FAIL] {url}: {e}")
        return None


def extract_article_links(html, base_url):
    """从列表页提取文章链接。"""
    soup = BeautifulSoup(html, 'html.parser')
    links = []

    # NMPA典型结构：<li><a href="...">标题</a></li>
    for a in soup.find_all('a', href=True):
        href = a['href']
        if not href.startswith(('http://', 'https://')):
            href = urljoin(base_url, href)

        # 过滤：只保留详情页
        if '/xxgk/' in href or '/kpzs/' in href or '.html' in href:
            title = a.get_text(strip=True)
            if len(title) > 8 and len(title) < 100:
                links.append((title, href))

    return links[:30]  # 每页最多30篇


def extract_body(html):
    """提取正文文本。"""
    soup = BeautifulSoup(html, 'html.parser')

    # 尝试多种常见选择器
    selectors = [
        '#articleContent',
        '.article-content',
        '.content',
        '#content',
        'article',
        '.TRS_Editor',  # 政府网站常用
    ]

    for sel in selectors:
        container = soup.select_one(sel)
        if container:
            text = container.get_text(separator='\n', strip=True)
            text = re.sub(r'\n\s*\n', '\n\n', text)
            zh_chars = len(re.findall(r'[\u4e00-\u9fff]', text))
            if zh_chars >= MIN_ZH_CHARS:
                return text

    # fallback：找所有<p>标签
    paragraphs = soup.find_all('p')
    if len(paragraphs) > 3:
        text = '\n'.join(p.get_text(strip=True) for p in paragraphs if len(p.get_text(strip=True)) > 20)
        zh_chars = len(re.findall(r'[\u4e00-\u9fff]', text))
        if zh_chars >= MIN_ZH_CHARS:
            return text

    return None


def download_article(title, url, idx):
    """下载单篇文章。"""
    safe_title = re.sub(r'[^\w\u4e00-\u9fff]', '_', title)[:40]
    filename = f"nmpa_{idx:03d}_{safe_title}.txt"
    filepath = ROOT / filename

    if filepath.exists() and filepath.stat().st_size > 500:
        return False

    html = fetch_html(url)
    if not html:
        return False

    body = extract_body(html)
    if not body:
        print(f"  [SHORT] {title}")
        return False

    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(f"标题: {title}\n")
        f.write(f"来源: 国家药品监督管理局\n")
        f.write(f"URL: {url}\n")
        f.write(f"日期: {time.strftime('%Y-%m-%d')}\n")
        f.write("=" * 60 + "\n\n")
        f.write(body)

    print(f"  [OK] {filepath.name} ({len(body)} 字符)")
    save_manifest_entry(url, title, time.strftime('%Y-%m-%d'))
    return True


def main():
    print("=" * 70)
    print("国家药监局药品安全科普批量下载器")
    print("=" * 70)

    existing = load_manifest()
    print(f"清单中已有 {len(existing)} 条记录\n")

    total_downloaded = 0

    for url in NMPA_URLS:
        print(f"处理: {url}")
        html = fetch_html(url)
        if not html:
            continue

        articles = extract_article_links(html, url)
        print(f"  发现 {len(articles)} 篇文章")

        for idx, (title, article_url) in enumerate(articles, 1):
            if article_url in existing:
                continue

            if download_article(title, article_url, idx):
                existing.add(article_url)
                total_downloaded += 1

            time.sleep(DELAY)

        print()

    print(f"{'='*70}")
    print(f"本次新增 {total_downloaded} 篇")
    print(f"累计 {len(existing)} 条记录")
    print(f"输出目录: {ROOT}")


if __name__ == '__main__':
    main()
