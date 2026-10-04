# internal/logging — 日志落盘

只有一个文件，`rotating_writer.go`：`RotatingWriter`（`NewRotatingWriter(dir, name, maxSizeMB, maxAgeDays)` / `Write` / `Close`），按大小轮转 + 按天数清理。

唯一调用点：`main.go:61`，`logging.NewRotatingWriter("logs", "doctor-agent.log", 10, 7)` —— 即 10MB 一份、保留 7 天，写到工作目录下的 `logs/`（已 gitignore）。

约定不变：**日志一律 `slog`**（`slog.Info` 生命周期、`slog.Debug` 细节、`slog.Warn` 安全层命中），不引 logrus/zap。本包只负责输出目的地，不负责字段与级别策略。
