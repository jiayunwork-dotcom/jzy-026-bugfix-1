# camkin — 凸轮从动件运动学小服务

只做凸轮从动件运动学：点名一条运动规律（余弦/简谐、摆线、等加速等减速），
给出升程 h、升程角 β、角速度 ω，服务沿转角输出位移 s、速度 v、加速度 a、跃度 j，
并核对接头处该连续的量是否连续；完整一周可由升程、远休止、回程、近休止四段拼成循环曲线。
仅经 HTTP 提供规律档与曲线，配方与循环档保存在本地 JSON 文件。

## 无因次规律与量纲还原

无因次时间 T = θ/β，升程段内 T ∈ [0,1]，无量纲位移 S = s/h：

| 类型 | S(T) | S′(T) | S″(T) | 备注 |
|---|---|---|---|---|
| `cosine` 余弦（简谐） | (1−cos πT)/2 | π/2·sin πT | π²/2·cos πT | 两端 v=0，两端 a≠0（±π²/2·h(ω/β)²） |
| `cycloid` 摆线 | T−sin(2πT)/(2π) | 1−cos(2πT) | 2π·sin(2πT) | 两端 v=0、a=0 |
| `parabolic` 等加速等减速 | T≤1/2: 2T²；T>1/2: 1−2(1−T)² | 4T / 4(1−T) | +4 / −4 | 中点切换；两端 v=0、a≠0；中点 a 变号、s/v 连续；中点 j 为理想冲激 |

s、v、a、j 全部由同一套解析式对转角求导，再按 `d/dt = (ω/β)·d/dT` 还原量纲，
不使用相邻点差分：

```
s = h·S
v = h·(ω/β)·S′
a = h·(ω/β)²·S″
j = h·(ω/β)³·S‴
```

θ=0 时 s=0，θ=β 时 s=h；采样网格端点直接取解析端值，保证网格与闭式严格一致。

解析峰值（绝对值）：

- 余弦：v_max=(π/2)hω/β，a_max=(π²/2)h(ω/β)²，j_max=(π³/2)h(ω/β)³
- 摆线：v_max=2hω/β，a_max=2πh(ω/β)²，j_max=4π²h(ω/β)³
- 等加速：v_max=2hω/β，a_max=4h(ω/β)²；j 在中点为冲激，无有界峰值

回程是同一局部转角上对位移的镜像：s_ret(θ)=h−s_rise(θ)，v/a/j 反号。
停歇段 s 保持 0 或 h，v、a、j 全为 0。停歇角为 0 表示该段省略。

## 角度单位

角度单位全程使用同一套，钉死为 `degree` 或 `radian`，并在每份曲线结果的
`angle_unit` 字段写明；β 必须为正且小于一周（度 < 360，弧度 < 2π）。
配方以度登记 β，请求时可用 `unit=radian` 换算后计算（弧度下仍再次校验 < 2π）。
ω 与 β 必须同用度/秒或弧度/秒。

## 约束

- h 必须为正；β 必须为正且小于一周；ω 必须为正；未知规律名在求曲线前直接拒绝。
- 循环档四段角度之和必须恰好等于一周（数值容差 1e-9）。
- 循环拼接角位移必须连续；任一接头出现位移跳变则整周构建失败。
- 只把 h 加大，s、v、a、j 同比例放大；只把 β 加大（ω 不变），峰值速度下降。

## HTTP 接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/laws` | 规律目录：类型、无因次端值、解析峰值、性质说明 |
| GET/POST | `/api/recipes` | 列出/登记配方（名字、类型、h、beta[度]） |
| GET/PUT/DELETE | `/api/recipes/{name}` | 配方增改查删 |
| GET | `/api/recipes/{name}/rise?omega=&unit=&n=` | 点名配方求升程曲线（含端点/等加速中点核对、解析峰值） |
| POST | `/api/rise` | 不登记，直接 body 指定 type/h/beta/omega/unit/n 求升程 |
| GET/POST | `/api/cycles` | 列出/登记循环档（四段角度与各段类型、h、unit） |
| GET/PUT/DELETE | `/api/cycles/{name}` | 循环档增改查删 |
| GET | `/api/cycles/{name}/curve?omega=&step=` | 求一周曲线（points、各段区间、接头核对、观测峰值） |

登记示例：

```bash
curl -X POST localhost:8080/api/recipes \
  -H 'Content-Type: application/json' \
  -d '{"name":"follower_a","type":"cycloid","h":10,"beta":120}'

curl 'localhost:8080/api/recipes/follower_a/rise?omega=6&unit=degree&n=360'
```

循环档示例（四段和必须为 360 度；停歇角 0 即省略）：

```bash
curl -X POST localhost:8080/api/cycles \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"full", "unit":"degree", "h":10,
    "rise":{"angle":120,"type":"parabolic"},
    "far_dwell":{"angle":60},
    "return":{"angle":90,"type":"cycloid"},
    "near_dwell":{"angle":90}
  }'

curl 'localhost:8080/api/cycles/full/curve?omega=6&step=1'
```

启动时若配方目录为空，自动载入默认配方 `default_cycloid`（摆线升程，h=10，β=120 度，
两端加速度为 0，终点位移等于 h）。

## 源码组织

| 文件 | 职责 |
|---|---|
| `internal/laws/laws.go` | 无因次规律 S 及其对 T 的各阶导数、端值、无量纲峰值 |
| `internal/kinematics/kinematics.go` | 乘 ω 还原量纲、参数校验、升/回程解析状态、解析峰值、采样 |
| `internal/kinematics/rise.go` | 升程曲线组装与端点核对、等加速中点接头核对 |
| `internal/cycle/cycle.go` | 四段循环拼接、网格生成、接头连续性核对（含闭环接头） |
| `internal/store/store.go` | 配方与循环档的本地 JSON 文件读写、启动播种 |
| `internal/httpapi/server.go` | HTTP 路由与请求校验 |
| `cmd/camkin/main.go` | 入口（`PORT` 默认 8080，`DATA_DIR` 默认 ./data） |

## 构建与运行

```bash
go test ./...
go run ./cmd/camkin

# 容器（golang:1.22-alpine 构建，单容器）
docker build -t camkin .
docker run --rm -p 8080:8080 -v $PWD/data:/data camkin
```

测试锁定：升程终点 s=h、摆线两端 a=0、等加速中点 a 变号且终点仍为 h、
等加速两端 a≠0、余弦两端 v=0、h 加大曲线等比放大、β 加大峰值速度下降、
未知规律被拒、接头位移连续、停歇段导数为 0。
