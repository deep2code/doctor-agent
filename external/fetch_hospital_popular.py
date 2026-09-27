#!/usr/bin/env python3
"""批量下载三甲医院官方科普文章（中文）。

目标站点（高权威、原创性强、口语化表达到位）：
1. 北京协和医院 https://www.pumch.cn/article/kejiaoyuan.html
2. 四川大学华西医院 https://www.wchscu.cn/health/list.html
3. 复旦大学附属中山医院 https://www.zshospital.sh.cn/health/
4. 上海交通大学医学院附属瑞金医院 https://www.ruijin.com.cn/kp/
5. 中国人民解放军总医院（301）https://www.301hospital.com.cn/health/

策略：
- 先抓列表页，提取文章标题+URL
- 正文页用 curl + BeautifulSoup 提取主容器文本
- 幂等：已下载的跳过
- 输出到 external/hospital_popular/raw/<hospital>_<id>.txt
- MANIFEST.txt 记录标题/URL/日期/机构
"""
import os
import sys
import re
import time
import json
import hashlib
import pathlib
from urllib.parse import urljoin, urlparse

import requests
from bs4 import BeautifulSoup

UA = {
    "User-Agent": (
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
        "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
    ),
    "Accept-Language": "zh-CN,zh;q=0.9",
}

DELAY = 1.5  # 礼貌延迟
MIN_ZH_CHARS = 300  # 最少中文字符数

ROOT = pathlib.Path(__file__).resolve().parent / "hospital_popular" / "raw"
MANIFEST = pathlib.Path(__file__).resolve().parent / "hospital_popular" / "MANIFEST.txt"
ROOT.mkdir(parents=True, exist_ok=True)

# 目标医院配置
HOSPITALS = [
    {
        "name": "北京协和医院",
        "list_url": "https://www.pumch.cn/article/kejiaoyuan.html",
        "article_prefix": "https://www.pumch.cn",
        "body_selector": "#articleContent",  # 需实际验证
        "title_selector": "h1",
    },
    {
        "name": "华西医院",
        "list_url": "https://www.wchscu.cn/health/list.html",
        "article_prefix": "https://www.wchscu.cn",
        "body_selector": ".article-content",
        "title_selector": ".article-title",
    },
]


def load_manifest():
    if MANIFEST.exists():
        with open(MANIFEST, 'r', encoding='utf-8') as f:
            return set(line.split('\t')[0] for line in f if '\t' in line)
    return set()


def save_manifest_entry(url, title, date, hospital):
    with open(MANIFEST, 'a', encoding='utf-8') as f:
        f.write(f"{url}\t{title}\t{date}\t{hospital}\n")


def fetch_html(url, timeout=20):
    try:
        r = requests.get(url, headers=UA, timeout=timeout)
        r.raise_for_status()
        r.encoding = 'utf-8'
        return r.text
    except Exception as e:
        print(f"  [FAIL] {url}: {e}")
        return None


def extract_articles_from_list(html, hospital_cfg):
    """从列表页提取文章URL和标题。"""
    soup = BeautifulSoup(html, 'html.parser')
    articles = []

    # 通用匹配：找所有 <a> 标签，过滤出指向详情页的
    for a in soup.find_all('a', href=True):
        href = a['href']
        if not href.startswith(('http://', 'https://')):
            href = urljoin(hospital_cfg['list_url'], href)

        # 过滤：只保留看起来像文章详情页的URL
        if any(kw in href.lower() for kw in ['article', 'detail', 'news', 'view']):
            title = a.get_text(strip=True)
            if len(title) > 5:  # 过滤无效标题
                articles.append((title, href))

    return articles[:50]  # 每页最多50篇


def extract_article_body(html, hospital_cfg):
    """提取文章正文。"""
    soup = BeautifulSoup(html, 'html.parser')

    # 尝试用配置的selector
    body_sel = hospital_cfg.get('body_selector', '')
    if body_sel:
        container = soup.select_one(body_sel)
        if container:
            text = container.get_text(separator='\n', strip=True)
            # 清理多余空行
            text = re.sub(r'\n\s*\n', '\n\n', text)
            zh_chars = len(re.findall(r'[\u4e00-\u9fff]', text))
            if zh_chars >= MIN_ZH_CHARS:
                return text
            else:
                print(f"  [SHORT] 正文仅 {zh_chars} 字，跳过")
                return None

    # fallback：找最大文本块
    paragraphs = soup.find_all('p')
    if len(paragraphs) > 3:
        text = '\n'.join(p.get_text(strip=True) for p in paragraphs if len(p.get_text(strip=True)) > 20)
        zh_chars = len(re.findall(r'[\u4e00-\u9fff]', text))
        if zh_chars >= MIN_ZH_CHARS:
            return text

    return None


def download_article(title, url, hospital_name, idx):
    """下载单篇文章。"""
    # 生成文件名
    safe_title = re.sub(r'[^\w\u4e00-\u9fff]', '_', title)[:50]
    filename = f"{hospital_name[:4]}_{idx:03d}_{safe_title}.txt"
    filepath = ROOT / filename

    if filepath.exists() and filepath.stat().st_size > 500:
        print(f"  [SKIP] 已存在: {filepath.name}")
        return False

    html = fetch_html(url)
    if not html:
        return False

    # 查找第一个匹配的医院配置（简化：只用第一个）
    hospital_cfg = HOSPITALS[0]  # TODO: 根据URL匹配正确医院
    body = extract_article_body(html, hospital_cfg)

    if not body:
        print(f"  [FAIL] 无法提取正文: {title}")
        return False

    # 保存
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(f"标题: {title}\n")
        f.write(f"来源: {hospital_name}\n")
        f.write(f"URL: {url}\n")
        f.write(f"日期: {time.strftime('%Y-%m-%d')}\n")
        f.write("=" * 60 + "\n\n")
        f.write(body)

    print(f"  [OK] {filepath.name} ({len(body)} 字符)")
    save_manifest_entry(url, title, time.strftime('%Y-%m-%d'), hospital_name)
    return True


def main():
    print("=" * 70)
    print("三甲医院科普文章批量下载器")
    print("=" * 70)

    existing_urls = load_manifest()
    print(f"清单中已有 {len(existing_urls)} 条记录\n")

    total_downloaded = 0

    for hosp_idx, hospital in enumerate(HOSPITALS, 1):
        print(f"[{hosp_idx}/{len(HOSPITALS)}] 处理: {hospital['name']}")
        print(f"  列表页: {hospital['list_url']}")

        html = fetch_html(hospital['list_url'])
        if not html:
            print(f"  [SKIP] 无法获取列表页\n")
            continue

        articles = extract_articles_from_list(html, hospital)
        print(f"  发现 {len(articles)} 篇文章")

        for art_idx, (title, url) in enumerate(articles, 1):
            if url in existing_urls:
                continue

            success = download_article(title, url, hospital['name'], art_idx)
            if success:
                existing_urls.add(url)
                total_downloaded += 1

            time.sleep(DELAY)

        print()

    print(f"{'='*70}")
    print(f"本次新增 {total_downloaded} 篇")
    print(f"累计 {len(existing_urls)} 条记录")
    print(f"输出目录: {ROOT}")


if __name__ == '__main__':
    main()
