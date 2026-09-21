package laws_test

import (
	"math"
	"testing"

	"camkin/internal/laws"
)

func TestEndpointValues(t *testing.T) {
	for _, typ := range laws.Types() {
		l, err := laws.Get(typ)
		if err != nil {
			t.Fatalf("取规律 %s 失败: %v", typ, err)
		}
		s0, v0, a0, _ := l.Eval(0)
		s1, v1, a1, _ := l.Eval(1)
		if math.Abs(s0) > 1e-15 {
			t.Errorf("%s: S(0)=%v, 期望 0", typ, s0)
		}
		if math.Abs(s1-1) > 1e-15 {
			t.Errorf("%s: S(1)=%v, 期望 1", typ, s1)
		}
		if math.Abs(v0) > 1e-15 || math.Abs(v1) > 1e-15 {
			t.Errorf("%s: 两端 V 应为 0，得到 %v, %v", typ, v0, v1)
		}
		_ = a0
		_ = a1
	}

	cycloid, _ := laws.Get(laws.Cycloid)
	c0, _, a0, _ := cycloid.Eval(0)
	c1, _, a1, _ := cycloid.Eval(1)
	if c0 != 0 || math.Abs(c1-1) > 1e-15 {
		t.Fatalf("摆线端点位移错误: %v %v", c0, c1)
	}
	// 摆线两端加速度也为 0（闭式 sin(0)=sin(2π)=0；数值求值允许微小残差）。
	if math.Abs(a0) > 1e-12 || math.Abs(a1) > 1e-12 {
		t.Errorf("摆线两端 A 应为 0，得到 %v, %v", a0, a1)
	}

	// 等加速两端加速度不为 0（+4、-4），测试必须反映这一已知性质。
	para, _ := laws.Get(laws.Parabolic)
	_, _, pa0, _ := para.Eval(0)
	_, _, pa1, _ := para.Eval(1)
	if pa0 != 4 || pa1 != -4 {
		t.Errorf("等加速两端 A 应分别为 +4、-4，得到 %v, %v", pa0, pa1)
	}
}

func TestParabolicMidpointSwitch(t *testing.T) {
	para, _ := laws.Get(laws.Parabolic)
	// 前半：T=0.5 恰好 S=0.5；后半必须在中点切换，否则终点对不上 1。
	sBefore, vBefore, aBefore, _ := para.Eval(0.5 - 1e-12)
	sAt, vAt, _, _ := para.Eval(0.5)
	sAfter, vAfter, aAfter, _ := para.Eval(0.5 + 1e-12)
	if math.Abs(sAt-0.5) > 1e-12 {
		t.Errorf("等加速中点 S=%v, 期望 0.5", sAt)
	}
	if math.Abs(sBefore-0.5) > 1e-9 || math.Abs(sAfter-0.5) > 1e-9 {
		t.Errorf("中点两侧位移应均为 0.5，得到 %v, %v", sBefore, sAfter)
	}
	if math.Abs(vBefore-2) > 1e-9 || math.Abs(vAfter-2) > 1e-9 || math.Abs(vAt-2) > 1e-12 {
		t.Errorf("中点速度应连续且为 2（S'=4T 在 T=1/2），得到 %v, %v, %v", vBefore, vAt, vAfter)
	}
	if !(aBefore > 0 && aAfter < 0) {
		t.Errorf("中点加速度应变号（+4 -> -4），得到 %v, %v", aBefore, aAfter)
	}
	sEnd, _, _, _ := para.Eval(1)
	if math.Abs(sEnd-1) > 1e-15 {
		t.Errorf("等加速终点 S=%v, 期望 1（不切换会对不上）", sEnd)
	}
}

func TestUnknownLawRejected(t *testing.T) {
	if _, err := laws.Get("modified_sine_plus"); err == nil {
		t.Fatal("未知规律名必须返回错误")
	}
}

func TestCycloidAnalyticValues(t *testing.T) {
	cycloid, _ := laws.Get(laws.Cycloid)
	// T=1/2：S=1/2，S'=2，S''=0。
	s, v, a, _ := cycloid.Eval(0.5)
	if math.Abs(s-0.5) > 1e-15 {
		t.Errorf("摆线中点 S=%v 期望 0.5", s)
	}
	if math.Abs(v-2) > 1e-15 {
		t.Errorf("摆线 Vmax 无量纲值应为 2，得到 %v", v)
	}
	if math.Abs(a) > 1e-15 {
		t.Errorf("摆线中点 A 应为 0，得到 %v", a)
	}
	if math.Abs(cycloid.Peaks.A-2*math.Pi) > 1e-15 {
		t.Errorf("摆线 Amax 应为 2π，得到 %v", cycloid.Peaks.A)
	}
	if math.Abs(cycloid.Peaks.J-4*math.Pi*math.Pi) > 1e-12 {
		t.Errorf("摆线 Jmax 应为 4π²，得到 %v", cycloid.Peaks.J)
	}
}
