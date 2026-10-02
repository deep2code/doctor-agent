#!/bin/sh
# 让「镜像即数据」在任何挂载方式下都成立。
#
# 背景: mariadb 官方 entrypoint 只在数据目录为空时才导入
# /docker-entrypoint-initdb.d/ 里的 dump。而 docker compose 重建容器时会【复用】
# 上一个容器留下的匿名卷（实测如此），所以"不挂持久卷"本身并不能保证重新导入 ——
# 镜像更新了，容器里还是旧知识。那正是这个部署形态要消灭的状态。
#
# 因此本容器把数据目录当作"镜像里那份 dump 的一次解包结果"：每次启动先清掉上一次
# 的导入，再交给官方 entrypoint 重新导入。数据目录里没有任何独有内容，删了不心疼。
#
# 安全边界: 只清【自己打过标记】的目录。万一有人把一个不属于本镜像的卷挂到这里
# （例如误挂了业务库的卷），绝不删别人的数据 —— 原样启动并大声告警。
set -e

DATA=/var/lib/mysql
MARKER="$DATA/.doctor-agent-kb-imported"

if [ -f "$MARKER" ]; then
	echo "[doctor-agent-kb] 清空上次导入的数据目录，将从镜像重新导入 (约 40-50 秒)"
	rm -rf "${DATA:?}"/*
	rm -f "$DATA"/.[!.]* "$DATA"/..?* 2>/dev/null || true
elif [ -d "$DATA/mysql" ] || [ -f "$DATA/ibdata1" ]; then
	echo "[doctor-agent-kb] 警告: 挂载了一个不是本镜像导入的数据目录（没有标记文件）。" \
		"不会删除它，但知识内容可能已经落后于镜像 —— 这是部署配置问题，请把 kb 服务的持久卷去掉。"
fi

exec /usr/local/bin/docker-entrypoint.sh "$@"
