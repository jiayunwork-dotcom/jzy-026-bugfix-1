package kinematics_test

import (
	"math"
	"testing"

	"camkin/internal/kinematics"
	"camkin/internal/laws"
)

func baseParams() kinematics.Params {
	return kinematics.Params{H: 10, Beta: 120, Omega: 6, Unit: kinematics.Degree}
}

func approx(a, b, relTol float64) bool {
	return math.Abs(a-b) <= relTol*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

// 升程终点 s=h、起点 s=0；全部规律、两种角度单位。
func TestRiseEndpoints(t *testing.T) {
	for _, unit := range []string{kinematics.Degree, kinematics.Radian} {
		for _, typ := range laws.Types() {
			law, _ := laws.Get(typ)
			p := baseParams()
			p.Unit = unit
			if unit == kinematics.Radian {
				p.Beta = 120 * math.Pi / 180
			}
			res, err := kinematics.RiseCurve(law, p, 240)
			if err != nil {
				t.Fatalf("%s(%s): %v", typ, unit, err)
			}
			if res.Start.S != 0 {
				t.Errorf("%s(%s): s(0)=%v 期望 0", typ, unit, res.Start.S)
			}
			if res.End.V != 0 {
				t.Errorf("%s(%s): v(beta)=%v 期望 0", typ, unit, res.End.V)
			}
			if !approx(res.End.S, p.H, 1e-12) {
				t.Errorf("%s(%s): s(beta)=%v 期望 h=%v", typ, unit, res.End.S, p.H)
			}
			last := res.Points[len(res.Points)-1]
			if !approx(last.S, p.H, 1e-12) {
				t.Errorf("%s(%s): 网格末端 s=%v 与闭式 h=%v 对不上", typ, unit, last.S, p.H)
			}
		}
	}
}

// 摆线两端加速度为 0（有量纲）。
func TestCycloidEndpointAccelerationZero(t *testing.T) {
	law, _ := laws.Get(laws.Cycloid)
	res, err := kinematics.RiseCurve(law, baseParams(), 240)
	if err != nil {
		t.Fatal(err)
	}
	if res.Start.A != 0 || res.End.A != 0 {
		t.Errorf("摆线两端 a 应为 0，得到 %v, %v", res.Start.A, res.End.A)
	}
}

// 余弦两端速度为 0，加速度不为 0。
func TestCosineEndpointVelocity(t *testing.T) {
	law, _ := laws.Get(laws.Cosine)
	p := baseParams()
	res, err := kinematics.RiseCurve(law, p, 240)
	if err != nil {
		t.Fatal(err)
	}
	if res.Start.V != 0 || res.End.V != 0 {
		t.Errorf("余弦两端 v 应为 0，得到 %v, %v", res.Start.V, res.End.V)
	}
	wantA := p.H * (p.Omega / p.Beta) * (p.Omega / p.Beta) * math.Pi * math.Pi / 2
	if !approx(math.Abs(res.Start.A), wantA, 1e-12) {
		t.Errorf("余弦起点 |a|=%v 期望 π²/2·h(ω/β)²=%v", res.Start.A, wantA)
	}
}

// 等加速：两端速度为 0、加速度不为 0；中点变号；终点仍为 h。
func TestParabolicProperties(t *testing.T) {
	law, _ := laws.Get(laws.Parabolic)
	p := baseParams()
	res, err := kinematics.RiseCurve(law, p, 240)
	if err != nil {
		t.Fatal(err)
	}
	if res.Start.V != 0 || res.End.V != 0 {
		t.Errorf("等加速两端 v 应为 0，得到 %v, %v", res.Start.V, res.End.V)
	}
	if res.Start.A <= 0 || res.End.A >= 0 {
		t.Fatalf("等加速两端 a 应分别为正、负且不为 0，得到 %v, %v", res.Start.A, res.End.A)
	}
	if len(res.InternalJunctions) != 1 {
		t.Fatalf("等加速应有 1 个内部接头（中点），得到 %d", len(res.InternalJunctions))
	}
	mid := res.InternalJunctions[0]
	if !mid.SContinuous || !mid.VContinuous {
		t.Errorf("等加速中点位移、速度必须连续: %+v", mid)
	}
	if !mid.ASignChange {
		t.Errorf("等加速中点加速度必须变号: %+v", mid)
	}
	if !approx(mid.Left.S, p.H/2, 1e-9) || !approx(mid.Left.V, 2*p.H*p.Omega/p.Beta, 1e-9) {
		t.Errorf("等加速中点 s=h/2、v=2hω/β，得到 %+v", mid)
	}
	if !approx(res.End.S, p.H, 1e-12) {
		t.Errorf("等加速终点 s=%v 期望 h=%v", res.End.S, p.H)
	}
}

// 只把 h 加大，全程 s、v、a、j 按同一比例放大。
func TestScalingWithH(t *testing.T) {
	for _, typ := range laws.Types() {
		law, _ := laws.Get(typ)
		p1 := baseParams()
		p2 := p1
		p2.H = 2 * p1.H
		r1, _ := kinematics.RiseCurve(law, p1, 100)
		r2, _ := kinematics.RiseCurve(law, p2, 100)
		if len(r1.Points) != len(r2.Points) {
			t.Fatalf("%s: 网格长度不一致", typ)
		}
		for i := range r1.Points {
			q1, q2 := r1.Points[i], r2.Points[i]
			for k, got := range [4]float64{q2.S, q2.V, q2.A, q2.J} {
				want := 2 * [4]float64{q1.S, q1.V, q1.A, q1.J}[k]
				if !approx(got, want, 1e-9) {
					t.Errorf("%s 点 %d 阶 %d: h 加倍后 %v 应等比放大为 %v", typ, i, k, got, want)
				}
			}
		}
	}
}

// 只把 beta 加大、omega 不变，峰值速度下降。
func TestPeakVelocityDecreasesWithBeta(t *testing.T) {
	for _, typ := range laws.Types() {
		law, _ := laws.Get(typ)
		p1 := baseParams()
		p2 := p1
		p2.Beta = 180
		r1, _ := kinematics.RiseCurve(law, p1, 360)
		r2, _ := kinematics.RiseCurve(law, p2, 360)
		if !(r2.ObservedPeaks.V < r1.ObservedPeaks.V) {
			t.Errorf("%s: beta 加大后峰值速度应下降，%v -> %v", typ,
				r1.ObservedPeaks.V, r2.ObservedPeaks.V)
		}
		// 解析峰值应与 v_max 系数·hω/β 一致。
		want1 := law.Peaks.V * p1.H * p1.Omega / p1.Beta
		if !approx(r1.AnalyticPeaks.V, want1, 1e-12) {
			t.Errorf("%s 解析 vmax=%v 期望 %v", typ, r1.AnalyticPeaks.V, want1)
		}
	}
}

// 摆线峰值加速度、峰值跃度必须对得上由 h、beta、omega 写出的闭式，
// 且网格上观测到的峰值不超过解析峰值。
func TestCycloidClosedFormPeaks(t *testing.T) {
	law, _ := laws.Get(laws.Cycloid)
	p := baseParams()
	res, _ := kinematics.RiseCurve(law, p, 1000)
	k := p.Omega / p.Beta
	wantV := 2 * p.H * k
	wantA := 2 * math.Pi * p.H * k * k
	wantJ := 4 * math.Pi * math.Pi * p.H * k * k * k
	if !approx(res.AnalyticPeaks.V, wantV, 1e-12) {
		t.Errorf("摆线 v_max=%v 闭式=%v", res.AnalyticPeaks.V, wantV)
	}
	if !approx(res.AnalyticPeaks.A, wantA, 1e-12) {
		t.Errorf("摆线 a_max=%v 闭式=%v", res.AnalyticPeaks.A, wantA)
	}
	if !approx(res.AnalyticPeaks.J, wantJ, 1e-12) {
		t.Errorf("摆线 j_max=%v 闭式=%v", res.AnalyticPeaks.J, wantJ)
	}
	if res.ObservedPeaks.A > wantA*(1+1e-9) {
		t.Errorf("观测 a 峰值 %v 超过闭式 %v", res.ObservedPeaks.A, wantA)
	}
	if res.ObservedPeaks.J > wantJ*(1+1e-9) {
		t.Errorf("观测 j 峰值 %v 超过闭式 %v", res.ObservedPeaks.J, wantJ)
	}
}

func TestReturnMirror(t *testing.T) {
	for _, typ := range laws.Types() {
		law, _ := laws.Get(typ)
		p := baseParams()
		st := kinematics.ReturnStateAt(law, p, 0)
		en := kinematics.ReturnStateAt(law, p, p.Beta)
		if st.S != p.H {
			t.Errorf("%s 回程起点 s=%v 期望 h", typ, st.S)
		}
		if !approx(en.S, 0, 1e-12) {
			t.Errorf("%s 回程终点 s=%v 期望 0", typ, en.S)
		}
		if st.V != 0 || en.V != 0 {
			t.Errorf("%s 回程两端 v 应为 0，得到 %v, %v", typ, st.V, en.V)
		}
		// 回程中段状态应是同一局部转角升程状态的位移镜像、导数反号。
		mid := p.Beta * 0.37
		got := kinematics.ReturnStateAt(law, p, mid)
		rise := kinematics.RiseStateAt(law, p, mid)
		if !approx(got.S, p.H-rise.S, 1e-12) ||
			!approx(got.V, -rise.V, 1e-12) ||
			!approx(got.A, -rise.A, 1e-12) ||
			!approx(got.J, -rise.J, 1e-12) {
			t.Errorf("%s 回程镜像错误: got=%+v rise=%+v", typ, got, rise)
		}
	}
}

func TestParamValidation(t *testing.T) {
	bad := []kinematics.Params{
		{H: 0, Beta: 90, Omega: 1, Unit: kinematics.Degree},
		{H: -1, Beta: 90, Omega: 1, Unit: kinematics.Degree},
		{H: 1, Beta: 0, Omega: 1, Unit: kinematics.Degree},
		{H: 1, Beta: 360, Omega: 1, Unit: kinematics.Degree},
		{H: 1, Beta: 400, Omega: 1, Unit: kinematics.Degree},
		{H: 1, Beta: 2 * math.Pi, Omega: 1, Unit: kinematics.Radian},
		{H: 1, Beta: 1, Omega: 0, Unit: kinematics.Degree},
		{H: 1, Beta: 1, Omega: -2, Unit: kinematics.Degree},
		{H: 1, Beta: 1, Omega: 1, Unit: "grad"},
	}
	for i, p := range bad {
		if err := p.Validate(); err == nil {
			t.Errorf("非法参数组 %d 应被拒绝: %+v", i, p)
		}
	}
	// 弧度制下接近一周的角度换算成度合法（小于 360 度即可）。
	ok := kinematics.Params{H: 1, Beta: 6, Omega: 1, Unit: kinematics.Radian}
	if err := ok.Validate(); err != nil {
		t.Errorf("合法参数被拒: %v", err)
	}
}
