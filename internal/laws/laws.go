// Package laws 定义无因次从动件运动规律。
//
// 无因次时间 T = theta / beta，升程段内 T ∈ [0,1]。
// 本包只给出关于 T 的无量纲量及其对 T 的各阶导数，
// 量纲还原（乘以 h、omega 的相应次方）在 kinematics 包完成。
package laws

import (
	"fmt"
	"math"
)

// Law 是一条无因次规律。S 为无量纲位移 S=s/h，
// D1..D3 为 S 对无因次时间 T 的一、二、三阶导数。
type Law struct {
	Type string `json:"type"`
	Name string `json:"name"`
	// Eval 返回 (S, dS/dT, d²S/dT², d³S/dT³)。T 被夹到 [0,1]。
	Eval func(t float64) (s, d1, d2, d3 float64)
	// Endpoints 给出 T=0、T=1 处的解析端值，用于接头连续性核对。
	Endpoints [2]Values
	// Peaks 为各阶无量纲量的解析峰值（绝对值最大）。
	Peaks Peaks
}

// Values 是某一时刻的无量纲量。
type Values struct {
	S  float64 `json:"s"`
	V  float64 `json:"v"`
	A  float64 `json:"a"`
	J  float64 `json:"j"`
}

// Peaks 为无量纲解析峰值。
type Peaks struct {
	V float64 `json:"v"`
	A float64 `json:"a"`
	J float64 `json:"j"`
	// JBounded 为 false 表示跃度峰值无界（等加速中点为理想冲激），此时 J 记 0。
	JBounded bool `json:"j_bounded"`
}

const (
	// Cosine 余弦（简谐）运动：S=(1-cos(πT))/2。
	Cosine = "cosine"
	// Cycloid 摆线运动：S=T-sin(2πT)/(2π)。
	Cycloid = "cycloid"
	// Parabolic 等加速等减速运动：前半按 2T² 升，后半对称减速。
	Parabolic = "parabolic"
)

var registry = map[string]Law{
	Cosine: {
		Type: Cosine,
		Name: "余弦（简谐）",
		Eval: evalCosine,
		// 两端 S 为 0/1、V 为 0；A 在两端不为 0（±π²/2）。
		Endpoints: [2]Values{
			{S: 0, V: 0, A: -math.Pi * math.Pi / 2, J: 0},
			{S: 1, V: 0, A: +math.Pi * math.Pi / 2, J: 0},
		},
		// Vmax=π/2，Amax=π²/2，Jmax=π³/2。
		Peaks: Peaks{V: math.Pi / 2, A: math.Pi * math.Pi / 2, J: math.Pi * math.Pi * math.Pi / 2, JBounded: true},
	},
	Cycloid: {
		Type: Cycloid,
		Name: "摆线",
		Eval: evalCycloid,
		// 两端 S 为 0/1，V 与 A 均为 0。
		Endpoints: [2]Values{
			{S: 0, V: 0, A: 0, J: 4 * math.Pi * math.Pi},
			{S: 1, V: 0, A: 0, J: 4 * math.Pi * math.Pi},
		},
		// Vmax=2，Amax=2π，Jmax=4π²。
		Peaks: Peaks{V: 2, A: 2 * math.Pi, J: 4 * math.Pi * math.Pi, JBounded: true},
	},
	Parabolic: {
		Type: Parabolic,
		Name: "等加速等减速",
		Eval: evalParabolic,
		// 两端 S 为 0/1、V 为 0；A 不为 0（±4），这是该规律的已知性质。
		// J 在两段内部恒为 0，中点切换处为理想冲激，闭式曲线不在此给出冲激值。
		Endpoints: [2]Values{
			{S: 0, V: 0, A: 4, J: 0},
			{S: 1, V: 0, A: -4, J: 0},
		},
		// Vmax=2（中点：S'=4T 在 T=1/2 为 2），Amax=4；
		// J 在两段内部恒为 0，中点为理想冲激、无有界峰值。
		Peaks: Peaks{V: 2, A: 4, J: 0, JBounded: false},
	},
}

// Get 按名字取规律；未知名字返回错误（调用方应据此拒绝，绝不退化为差分）。
func Get(typ string) (Law, error) {
	if l, ok := registry[typ]; ok {
		return l, nil
	}
	return Law{}, fmt.Errorf("未知规律类型 %q，可选：%v", typ, Types())
}

// Types 列出已登记的规律类型。
func Types() []string {
	return []string{Cosine, Cycloid, Parabolic}
}

// Summary 是规律目录里一条规律的对外描述。
type Summary struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Start    Values `json:"start"`
	End      Values `json:"end"`
	Peaks    Peaks  `json:"peaks_dimensionless"`
	Note     string `json:"note"`
}

// Catalog 返回全部规律的目录。
func Catalog() []Summary {
	notes := map[string]string{
		Cosine:    "两端速度为0；两端加速度不为0（±π²/2）",
		Cycloid:   "两端速度、加速度均为0",
		Parabolic: "两端速度为0、加速度不为0（±4）；中点加速度变号、速度连续；中点跃度为理想冲激",
	}
	out := make([]Summary, 0, len(registry))
	for _, t := range Types() {
		l := registry[t]
		out = append(out, Summary{
			Type:  l.Type,
			Name:  l.Name,
			Start: l.Endpoints[0],
			End:   l.Endpoints[1],
			Peaks: l.Peaks,
			Note:  notes[t],
		})
	}
	return out
}

func clamp01(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

// 余弦（简谐）：
//
//	S   = (1-cos(πT))/2
//	S'  = π/2·sin(πT)
//	S'' = π²/2·cos(πT)
//	S'''= -π³/2·sin(πT)
func evalCosine(t float64) (s, d1, d2, d3 float64) {
	t = clamp01(t)
	p := math.Pi
	s = (1 - math.Cos(p*t)) / 2
	d1 = p / 2 * math.Sin(p*t)
	d2 = p * p / 2 * math.Cos(p*t)
	d3 = -p * p * p / 2 * math.Sin(p*t)
	return
}

// 摆线：
//
//	S    = T - sin(2πT)/(2π)
//	S'   = 1 - cos(2πT)
//	S''  = 2π·sin(2πT)
//	S''' = 4π²·cos(2πT)
func evalCycloid(t float64) (s, d1, d2, d3 float64) {
	t = clamp01(t)
	p2 := 2 * math.Pi
	s = t - math.Sin(p2*t)/p2
	d1 = 1 - math.Cos(p2*t)
	d2 = p2 * math.Sin(p2*t)
	d3 = p2 * p2 / 2 * math.Cos(p2*t) // 4π²·cos(2πT)
	return
}

// 等加速等减速（对称抛物线，前半与后半在 T=1/2 切换；不切换终点对不上 h）：
//
//	0≤T≤1/2: S=2T²,        S'=4T,  S''=4,  S'''=0
//	1/2<T≤1: S=1-2(1-T)²,  S'=4(1-T), S''=-4, S'''=0
//
// 两端速度为0；两端加速度分别为 +4、-4（不为0）；
// 中点两侧位移均为 1/2、速度均为 1（连续），加速度由 +4 翻为 -4。
func evalParabolic(t float64) (s, d1, d2, d3 float64) {
	t = clamp01(t)
	switch {
	case t <= 0.5:
		s = 2 * t * t
		d1 = 4 * t
		d2 = 4
	default:
		u := 1 - t
		s = 1 - 2*u*u
		d1 = 4 * u
		d2 = -4
	}
	d3 = 0
	return
}
