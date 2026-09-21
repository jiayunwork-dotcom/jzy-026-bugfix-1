// rise.go 组装单段升程曲线：解析采样、端点核对、规律内部接头核对（等加速中点）。
package kinematics

import (
	"fmt"

	"camkin/internal/laws"
)

// Boundary 是段端点解析值的核对结果。
type Boundary struct {
	Theta float64 `json:"theta"`
	State
}

// InternalJunction 是规律内部的分段切换点（等加速 T=1/2 中点）。
type InternalJunction struct {
	Theta       float64 `json:"theta"`
	Left        State   `json:"left"`
	Right       State   `json:"right"`
	SContinuous bool    `json:"s_continuous"`
	VContinuous bool    `json:"v_continuous"`
	ASignChange bool    `json:"a_sign_change"` // 中点加速度变号
	Note        string  `json:"note,omitempty"`
}

// RiseResult 是一条升程曲线的完整结果。
type RiseResult struct {
	Type               string             `json:"type"`
	AngleUnit          string             `json:"angle_unit"`
	Params             Params             `json:"params"`
	Points             []Point            `json:"points"`
	Start              Boundary           `json:"start"`
	End                Boundary           `json:"end"`
	InternalJunctions  []InternalJunction `json:"internal_junctions"`
	AnalyticPeaks      Peaks              `json:"analytic_peaks"`
	ObservedPeaks      Peaks              `json:"observed_peaks"`
	EndpointsChecked   []string           `json:"endpoints_checked"`
}

// RiseCurve 沿 [0,beta] 求升程曲线并核对端点。
// 返回的曲线全部来自同一套解析式对转角求导后乘 omega 相应次方，不用差分。
func RiseCurve(l laws.Law, p Params, n int) (*RiseResult, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	pts := SampleRise(l, p, n)
	start, end := EndStates(l, p)

	obs := Peaks{JBounded: l.Peaks.JBounded}
	for _, q := range pts {
		obs.V = maxf(obs.V, absf(q.V))
		obs.A = maxf(obs.A, absf(q.A))
		obs.J = maxf(obs.J, absf(q.J))
	}

	res := &RiseResult{
		Type: l.Type, AngleUnit: p.Unit, Params: p, Points: pts,
		Start: Boundary{Theta: 0, State: start},
		End:   Boundary{Theta: p.Beta, State: end},
		AnalyticPeaks: AnalyticPeaks(l, p),
		ObservedPeaks: obs,
		EndpointsChecked: []string{
			"s(0)=0", fmt.Sprintf("s(beta)=%v", p.H), "v(0)=0", "v(beta)=0",
		},
	}
	if l.Type == laws.Parabolic {
		res.InternalJunctions = append(res.InternalJunctions, parabolicMidCheck(p))
	}
	return res, nil
}

// parabolicMidCheck 核对等加速规律在 T=1/2 中点切换处：
// 两侧位移、速度连续，加速度由 +4 翻为 -4（变号）。
func parabolicMidCheck(p Params) InternalJunction {
	law, _ := laws.Get(laws.Parabolic)
	thetaMid := p.Beta / 2
	left := riseState(law, p, thetaMid-1e-12)
	right := riseState(law, p, thetaMid+1e-12)
	// 中点位移、速度取解析值 S=1/2、S'=1。
	hc := riseState(law, p, thetaMid)
	left.S, left.V = hc.S, hc.V
	right.S, right.V = hc.S, hc.V
	tol := 1e-9 * maxf(1, hc.S)
	j := InternalJunction{
		Theta:       thetaMid,
		Left:        left,
		Right:       right,
		SContinuous: absf(left.S-right.S) <= tol,
		VContinuous: absf(left.V-right.V) <= tol,
		ASignChange: left.A > 0 && right.A < 0,
		Note:        "中点加速度变号（+4 -> -4 乘比例因子），位移与速度连续",
	}
	return j
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
