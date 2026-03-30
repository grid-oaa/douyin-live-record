# douyin-live-record

单机版抖音直播自动录制服务，包含：

- Go 单体后端
- 内置管理页
- SQLite 配置与历史存储
- Streamlink 抓流
- FFmpeg 分段录制与合并
- Docker 化开发与部署

## 本地开发

1. 安装 Docker Desktop
2. 执行：

```bash
docker compose -f deploy/docker-compose.yml up --build
```

3. 浏览器访问 `http://localhost:8080/login`
4. 默认账号密码：

```text
admin / admin123456
```

## 生产构建

```bash
docker build -f deploy/Dockerfile -t douyin-live-record:latest .
docker run -d \
  --name douyin-live-record \
  -p 8080:8080 \
  -e APP_ADMIN_USERNAME=admin \
  -e APP_ADMIN_PASSWORD=change-me \
  -v /opt/douyin-recorder/data/db:/data/db \
  -v /opt/douyin-recorder/data/recordings:/data/recordings \
  -v /opt/douyin-recorder/cookies:/data/cookies \
  douyin-live-record:latest
```

## 页面功能

- `/login` 登录页
- `/admin/config` 录制配置
- `/admin/status` 当前状态和运行事件
- `/admin/history` 录制历史

## 关键说明

- 页面中的“保存子目录”始终是录制根目录 `/data/recordings` 下的子目录
- 录制过程先写入 `TS` 分段，直播结束后自动合并为 `MP4`
- `cookies_file` 需要填写相对 `/data/cookies` 的路径
- 首次启动会自动初始化管理员账号
