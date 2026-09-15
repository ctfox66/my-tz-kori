# Komari Monorepo

本仓库包含 Komari 服务端和 Agent：

- `komari/`：Komari 服务端与 Web 面板，基于 `1.2.5-fix2`
- `komari-agent/`：Komari Agent，基于 `1.2.13`

当前版本已移除 Web Shell、Web Terminal、远程命令执行和相关远程控制代码；Ping 监控功能仍然保留。

## 构建

```bash
cd komari
go test ./...
go build .

cd ../komari-agent
go test ./...
go build .
```

## 版本标签

- `1.2.5-fix2-server`
- `1.2.13-agent`
