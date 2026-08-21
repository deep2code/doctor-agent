import urllib.request
import os

base = '/Users/junjunyi/src-code/doctor-agent/external/kg/chinese-medical-kg/data'
os.makedirs(base, exist_ok=True)

files = {
    'icd10.xlsx': 'https://raw.githubusercontent.com/hint-lab/chinese-medical-kg/main/data/%E5%9B%BD%E5%AE%B6%E4%B8%B4%E5%BA%8A%E7%89%882.0%E7%96%BE%E7%97%85%E8%AF%8A%E6%96%AD%E7%BC%96%E7%A0%81%EF%BC%88ICD-10%EF%BC%89.xlsx',
    'drugs_domestic.xlsx': 'https://raw.githubusercontent.com/hint-lab/chinese-medical-kg/main/data/%E5%9B%BD%E5%AE%B6%E8%8D%AF%E5%93%81%E7%BC%96%E7%A0%81%E6%9C%AC%E4%BD%8D%E7%A0%81%E4%BF%A1%E6%81%AF%EF%BC%88%E5%9B%BD%E4%BA%A7%E8%8D%AF%E5%93%81%EF%BC%89.xlsx',
    'drugs_imported.xlsx': 'https://raw.githubusercontent.com/hint-lab/chinese-medical-kg/main/data/%E5%9B%BD%E5%AE%B6%E8%8D%AF%E5%93%81%E7%BC%96%E7%A0%81%E6%9C%AC%E4%BD%8D%E7%A0%81%E4%BF%A1%E6%81%AF%EF%BC%88%E8%BF%9B%E5%8F%A3%E8%8D%AF%E5%93%81%EF%BC%89.xlsx',
}

for name, url in files.items():
    path = os.path.join(base, name)
    urllib.request.urlretrieve(url, path)
    size = os.path.getsize(path)
    print(f'{name}: {size:,} bytes ({size/1024/1024:.1f} MB)')

print('Done')
