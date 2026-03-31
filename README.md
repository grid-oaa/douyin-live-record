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

## 启动命令

本地：

```cmd
docker run -d `
  --name douyin-live-record `
  -p 8080:8080 `
  -v D:\workspace\douyin-live-record:/workspace `
  -v D:\workspace\douyin-live-record\.local-test\db:/data/db `
  -v D:\workspace\douyin-live-record\.local-test\recordings:/data/recordings `
  -v D:\workspace\douyin-live-record\.local-test\cookies:/data/cookies `
  -e APP_LISTEN_ADDR=:8080 `
  -e APP_DB_PATH=/data/db/recorder.db `
  -e APP_RECORDINGS_ROOT=/data/recordings `
  -e APP_COOKIES_ROOT=/data/cookies `
  -e APP_ADMIN_USERNAME=admin `
  -e APP_ADMIN_PASSWORD=admin123456 `
  -e APP_SESSION_TTL_HOURS=168 `
  -e APP_PROBE_TIMEOUT_SECONDS=20 `
  -e APP_PROCESS_STOP_WAIT_SECONDS=8 `
  -w /workspace `
  douyin-live-record-dev:latest `
  /usr/local/go/bin/go run ./cmd/app
```

线上

~~~shell
docker run -d --name douyin-live-record --restart unless-stopped -p 8888:8080 -e APP_LISTEN_ADDR=:8080 -e APP_DB_PATH=/data/db/recorder.db -e APP_RECORDINGS_ROOT=/data/recordings -e APP_COOKIES_ROOT=/data/cookies -e APP_ADMIN_USERNAME=admin -e APP_ADMIN_PASSWORD='admin' -e APP_SESSION_TTL_HOURS=168 -e APP_PROBE_TIMEOUT_SECONDS=20 -e APP_PROCESS_STOP_WAIT_SECONDS=8 -v /project/douyin-live-record/db:/data/db -v /project/douyin-live-record/recordings:/data/recordings -v /project/douyin-live-record/cookies:/data/cookies douyin-live-record-dev:latest 
~~~

## 镜像构建

~~~sh
docker build -f deploy/Dockerfile.prod -t douyin-live-record:latest .

~~~



## 镜像保存

~~~sh
docker save -o douyin-live-record-latest.tar douyin-live-record:latest
~~~



## 镜像导入

~~~sh
docker load -i douyin-live-record-latest.tar

~~~

