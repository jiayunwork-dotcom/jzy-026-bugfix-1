// Package kinematics 把无因次规律乘以 h 与 omega 的相应次方，还原成有量纲的
// 位移 s、速度 v、加速度 a、跃度 j。
//
// 设无因次时间 T = theta/beta（theta 与 beta 必须使用同一角度单位），
// 则 d/dt = (omega/beta)·d/dT。角度单位在度与弧度之间保持一致即可：
// omega 与 beta 同用度/秒或同用弧度/秒，比值 omega/beta 的数值与单位制无关。
//
//	s = h·S
//	v = h·(omega/beta)·S'
//	a = h·(omega/beta)²·S''
//	j = h·(omega/beta)³·S'''
package kinematics

import (
	"fmt"
	"math"

	"camkin/internal/laws"
)

const (
	// Degree 表示角度单位为度。
	Degree = "degree"
	// Radian 表示角度单位为弧度。
	Radian = "radian"
)

// FullTurn 返回指定角度单位下一整周的角度。
func FullTurn(unit string) float64 {
	if unit == Radian {
		return 2 * math.Pi
	}
	return 360
}

// ValidUnit 判断角度单位是否被钉死为支持的两种之一。
func ValidUnit(unit string) bool {
	return unit == Degree || unit == Radian
}

// Params 是一段升程/回程运动的有量纲参数。
type Params struct {
	H     float64 // 升程（长度单位由调用方约定）
	Beta  float64 // 升程角（与 AngleUnit 一致）
	Omega float64 // 角速度（与 AngleUnit 一致，每秒）
	Unit  string  // 角度单位：degree 或 radian
}

// Validate 校验参数：h 必须为正；beta 必须为正且小于一周；omega 必须为正。
func (p Params) Validate() error {
	if !ValidUnit(p.Unit) {
		return fmt.Errorf("角度单位必须是 %q 或 %q，收到 %q", Degree, Radian, p.Unit)
	}
	if !(p.H > 0) {
		return fmt.Errorf("升程 h 必须为正，收到 %v", p.H)
	}
	if !(p.Beta > 0) {
		return fmt.Errorf("运动角 beta 必须为正，收到 %v", p.Beta)
	}
	if p.Beta >= FullTurn(p.Unit) {
		return fmt.Errorf("运动角 beta(%v %s) 必须小于一周(%v %s)",
			p.Beta, p.Unit, FullTurn(p.Unit), p.Unit)
	}
	if !(p.Omega > 0) {
		return fmt.Errorf("角速度 omega 必须为正，收到 %v", p.Omega)
	}
	return nil
}

// State 是某一转角处从动件的有量纲状态。
type State struct {
	S float64 `json:"s"` // 位移
	V float64 `json:"v"` // 速度 ds/dt
	A float64 `json:"a"` // 加速度 d²s/dt²
	J float64 `json:"j"` // 跃度 d³s/dt³
}

// Point 是曲线上的一个采样点。
type Point struct {
	Theta float64 `json:"theta"` // 转角（单位见响应中的 angle_unit）
	State
}

func scale(v laws.Values, h, k float64) State {
	return State{
		S: h * v.S,
		V: h * k * v.V,
		A: h * k * k * v.A,
		J: h * k * k * k * v.J,
	}
}

func mirror(q State) State {
	// 回程是升程对位移的镜像：s_ret = h - s_rise，各阶导数整体反号。
	return State{S: q.S, V: -q.V, A: -q.A, J: -q.J}
}

func riseState(l laws.Law, p Params, theta float64) State {
	t := theta / p.Beta
	return scale(ValuesOf(l, t), p.H, p.Omega/p.Beta)
}

// ValuesOf 是无因次规律的便捷取值，返回带导数标签的结构。
func ValuesOf(l laws.Law, t float64) laws.Values {
	s, d1, d2, d3 := l.Eval(t)
	return laws.Values{S: s, V: d1, A: d2, J: d3}
}

// RiseStateAt 求升程段内转角 theta 处的有量纲状态（解析闭式，非差分）。
// theta 被夹到 [0,beta]；落在端点时直接给解析端值，保证与闭式严格一致。
func RiseStateAt(l laws.Law, p Params, theta float64) State {
	start, end := EndStates(l, p)
	if theta <= 0 {
		return start
	}
	if theta >= p.Beta {
		return end
	}
	return riseState(l, p, theta)
}

// ReturnStateAt 求回程段内转角 theta（自回程起点计）处的有量纲状态。
func ReturnStateAt(l laws.Law, p Params, theta float64) State {
	q := RiseStateAt(l, p, theta)
	// 镜像位移：从 h 回到 0。
	q.S = p.H - q.S
	return mirror(q)
}

// EndStates 返回升程段起点（theta=0）与终点（theta=beta）的解析状态，
// 用于接头连续性核对，不依赖网格是否恰好落在端点。
func EndStates(l laws.Law, p Params) (start, end State) {
	k := p.Omega / p.Beta
	start = scale(l.Endpoints[0], p.H, k)
	end = scale(l.Endpoints[1], p.H, k)
	return
}

// ReturnEndStates 返回回程段起点与终点的解析状态。
// 回程对同一局部转角做位移镜像 s_ret(θ)=h-s_rise(θ)，各阶导数反号：
// 起点（θ=0）对应升程起点镜像 s=h，终点（θ=β）对应升程终点镜像 s=0。
func ReturnEndStates(l laws.Law, p Params) (start, end State) {
	rs, re := EndStates(l, p)
	start = mirror(rs)
	start.S = p.H - rs.S
	end = mirror(re)
	end.S = p.H - re.S
	return
}

// Peaks 为有量纲解析峰值（绝对值最大）。
type Peaks struct {
	V      float64 `json:"v_max"`
	A      float64 `json:"a_max"`
	J      float64 `json:"j_max"`
	JBounded bool   `json:"j_bounded"` // 等加速中点跃度为冲激，峰值无界
}

// AnalyticPeaks 返回由 h、beta、omega 写出的解析峰值。
//
// 以摆线为例：v_max=2hω/β，a_max=2πh(ω/β)²，j_max=4π²h(ω/β)³。
func AnalyticPeaks(l laws.Law, p Params) Peaks {
	k := p.Omega / p.Beta
	out := Peaks{JBounded: l.Peaks.JBounded}
	out.J = p.H * k * k * k * l.Peaks.J
	out.V = p.H * k * l.Peaks.V
	out.A = p.H * k * k * l.Peaks.A
	return out
}

// SampleRise 沿升程段转角 [0,beta] 均匀采样 n 个间隔（含两端，共 n+1 点）。
// 端点直接由解析端值给出，保证 s(0)=0、s(beta)=h 严格成立。
func SampleRise(l laws.Law, p Params, n int) []Point {
	if n < 1 {
		n = 1
	}
	pts := make([]Point, 0, n+1)
	for i := 0; i <= n; i++ {
		theta := p.Beta * float64(i) / float64(n)
		st := riseState(l, p, theta)
		if i == 0 {
			st, _ = EndStates(l, p)
		}
		if i == n {
			_, st = EndStates(l, p)
			theta = p.Beta
		}
		pts = append(pts, Point{Theta: theta, State: st})
	}
	return pts
}
