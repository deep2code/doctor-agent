#!/bin/sh
# 导入完成后打标记，供 kb-entrypoint.sh 判断"这个数据目录是本镜像解包出来的"。
# 文件名字典序排在 01-doctor_knowledge.sql.gz 之后，所以官方 entrypoint 一定是在
# 整份 dump 导完之后才跑到这里。
set -e
touch /var/lib/mysql/.doctor-agent-kb-imported
