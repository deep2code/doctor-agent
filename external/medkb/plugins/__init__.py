"""插件注册表：source 名 → 模块。__main__ 懒加载。"""

PLUGINS = {}


def load(source: str):
    if source in PLUGINS:
        return PLUGINS[source]
    import importlib
    mod = importlib.import_module(f".{source}", __package__)
    PLUGINS[source] = mod
    return mod


def all_sources() -> list[str]:
    from pathlib import Path
    pkg = Path(__file__).parent
    return sorted(p.stem for p in pkg.glob("*.py")
                  if p.stem not in ("base", "__init__"))
