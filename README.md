# Cost Board

Cost Board 是一个自托管的固定支出面板，用来记录订阅、服务器、域名等长期成本，并按汇率折算成年/月人民币支出。

它构建后只有一个二进制文件：后端、React 前端和静态资源都打包在一起。数据默认保存在 SQLite，保存前会自动写入最多 20 份 JSON 快照，方便误操作后手动恢复。

## 功能

- 用户名密码登录
- SQLite 存储，多设备访问同一份数据
- 添加、编辑、停用、删除固定支出项目
- 拖拽排序和分类汇总
- 外币按汇率折算为年度人民币
- 离线时显示浏览器里的本地缓存

## 快速部署

下面以 Linux 服务器为例。默认端口是 `8083`。

### 1. 下载并准备程序

从 GitHub Releases 下载适合服务器系统的文件，例如 `cost-board-linux-amd64`，然后放到服务器上：

```bash
sudo install -m 755 cost-board-linux-amd64 /usr/local/bin/cost-board
sudo mkdir -p /var/lib/cost-board
```

### 2. 设置登录账号

```bash
sudo cost-board --data-dir /var/lib/cost-board set-credentials
```

按提示输入用户名和密码。凭证会保存在 `/var/lib/cost-board/.auth`。

### 3. 先手动启动测试

```bash
sudo cost-board --host 127.0.0.1 --port 8083 --data-dir /var/lib/cost-board
```

如果服务器能正常启动，再按 `Ctrl+C` 停止，继续配置 systemd。

### 4. 配置 systemd

创建 `/etc/systemd/system/cost-board.service`：

```ini
[Unit]
Description=Cost Board
After=network.target

[Service]
ExecStart=/usr/local/bin/cost-board --host 127.0.0.1 --port 8083 --data-dir /var/lib/cost-board
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now cost-board
sudo systemctl status cost-board
```

## 反向代理

如果要通过域名访问，建议让 Cost Board 只监听 `127.0.0.1:8083`，再用 nginx 或 Caddy 对外提供 HTTPS。

### nginx 示例

把 `cost.example.com` 换成自己的域名：

```nginx
server {
    listen 80;
    server_name cost.example.com;

    location / {
        proxy_pass http://127.0.0.1:8083;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

启用后检查并重载：

```bash
sudo nginx -t
sudo systemctl reload nginx
```

如果使用 Certbot，可以再执行：

```bash
sudo certbot --nginx -d cost.example.com
```

### Caddy 示例

Caddy 会自动申请 HTTPS 证书。把下面内容加入 `Caddyfile`：

```caddyfile
cost.example.com {
    reverse_proxy 127.0.0.1:8083
}
```

然后重载：

```bash
sudo systemctl reload caddy
```

## Docker

如果你使用 Docker，可以直接运行镜像：

```bash
docker run -d --name cost-board \
  --restart unless-stopped \
  -p 8083:8083 \
  -v cost-board-data:/data \
  stqfdyr/cost-board:latest
```

首次启动后设置登录账号：

```bash
docker exec -it cost-board ./cost-board set-credentials
```

查看日志：

```bash
docker logs -f cost-board
```

升级到最新版本：

```bash
docker pull stqfdyr/cost-board:latest
docker stop cost-board
docker rm cost-board
docker run -d --name cost-board \
  --restart unless-stopped \
  -p 8083:8083 \
  -v cost-board-data:/data \
  stqfdyr/cost-board:latest
```

## 源码构建

```bash
git clone https://github.com/stqfdyr/cost-board.git
cd cost-board
make build
./cost-board --data-dir ./data set-credentials
./cost-board --port 8083 --data-dir ./data
```

## 配置

| Flag | Env | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--host` | `COST_BOARD_HOST` | `0.0.0.0` | 监听地址 |
| `--port` | `COST_BOARD_PORT` | `8083` | 监听端口 |
| `--data-dir` | `COST_BOARD_DATA_DIR` | `./data` | 数据目录 |

数据目录里主要包含：

- `cost-board.db`：SQLite 主数据库
- `.auth`：登录凭证
- `backups/`：保存前自动生成的 JSON 快照

## 常用命令

```bash
cost-board set-credentials          # 设置或修改用户名密码
cost-board import items.json        # 从 JSON 文件导入数据
cost-board --help                   # 查看帮助
```

## 开发

```bash
make dev    # Go 后端 :8083，Vite 前端 :5173
make build  # 构建前端并编译单二进制
```

## 技术栈

- Go + modernc.org/sqlite
- React + Vite + @dnd-kit
- scrypt 密码哈希 + 内存 session token

## License

MIT
