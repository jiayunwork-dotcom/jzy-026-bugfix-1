package cycle_test

import (
	"math"
	"testing"

	"camkin/internal/cycle"
)

func fullCycle(t *testing.T, riseType, retType string, far, near float64) cycle.Cycle {
	t.Helper()
	c := cycle.Cycle{
		Name: "test", Unit: "degree", H: 10,
		Rise:      cycle.MotionSeg{Angle: 120, Type: riseType},
		FarDwell:  cycle.DwellSeg{Angle: far},
		Return:    cycle.MotionSeg{Angle: 90, Type: retType},
		NearDwell: cycle.DwellSeg{Angle: near},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("循环档校验失败: %v", err)
	}
	return c
}

// 完整四段循环：拼接角位移必须连续，所有接头速度也连续。
func TestJunctionDisplacementContinuous(t *testing.T) {
	for _, typ := range []string{"cycloid", "cosine", "parabolic"} {
		c := fullCycle(t, typ, typ, 70, 80)
		res, err := cycle.Build(&c, 6, 0.5)
		if err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		if !res.Report.DisplacementContinuous {
			t.Errorf("%s: 存在位移跳变: %+v", typ, res.Report.Junctions)
		}
		if !res.Report.VelocityContinuous {
			t.Errorf("%s: 接头速度应连续（端点速度均为0）: %+v", typ, res.Report.Junctions)
		}
		for _, j := range res.Report.Junctions {
			if !j.SContinuous {
				t.Errorf("%s: 接头 θ=%v 位移跳变 %v -> %v",
					typ, j.Theta, j.Left.At.S, j.Right.At.S)
			}
		}
		// 首尾应闭合：第一点 s=0，最后一点 s=0。
		if res.Points[0].S != 0 {
			t.Errorf("%s: 一周起点 s=%v 应为 0", typ, res.Points[0].S)
		}
		if math.Abs(res.Points[len(res.Points)-1].S) > 1e-9 {
			t.Errorf("%s: 一周终点 s=%v 应为 0", typ, res.Points[len(res.Points)-1].S)
		}
		if math.Abs(res.Points[len(res.Points)-1].Theta-360) > 1e-9 {
			t.Errorf("网格终点 θ=%v 应为 360", res.Points[len(res.Points)-1].Theta)
		}
	}
}

// 停歇角为 0 表示该段省略，相邻两段位移不得接错。
func TestZeroDwellOmitted(t *testing.T) {
	// 两个停歇都为 0：120 + 0 + 240 + 0 = 360，升程直接接回程。
	c := cycle.Cycle{
		Name: "zero0", Unit: "degree", H: 10,
		Rise:      cycle.MotionSeg{Angle: 120, Type: "cycloid"},
		FarDwell:  cycle.DwellSeg{Angle: 0},
		Return:    cycle.MotionSeg{Angle: 240, Type: "cycloid"},
		NearDwell: cycle.DwellSeg{Angle: 0},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("循环档校验失败: %v", err)
	}
	res, err := cycle.Build(&c, 6, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, sg := range res.Segments {
		if sg.Kind == "dwell_far" || sg.Kind == "dwell_near" {
			t.Errorf("角度为 0 的停歇段不应出现在拼接结果: %+v", sg)
		}
	}
	// 升程结束角即回程开始角，两侧 s 都应是 h。
	riseEnd := res.Segments[0].End
	if riseEnd != 120 {
		t.Errorf("升程应在 θ=120 结束，得到 %v", riseEnd)
	}
	var j *cycle.Junction
	for i := range res.Report.Junctions {
		if res.Report.Junctions[i].Theta == 120 {
			j = &res.Report.Junctions[i]
		}
	}
	if j == nil {
		t.Fatal("未找到升程-回程接头")
	}
	if math.Abs(j.Left.At.S-c.H) > 1e-9 || math.Abs(j.Right.At.S-c.H) > 1e-9 {
		t.Errorf("省略远休止后接头两侧位移都应为 h，得到 %v -> %v", j.Left.At.S, j.Right.At.S)
	}

	// 只省略近休止：末段回程终点直接闭环回 θ=0 的 s=0。
	c2 := cycle.Cycle{
		Name: "zero1", Unit: "degree", H: 10,
		Rise:      cycle.MotionSeg{Angle: 120, Type: "cycloid"},
		FarDwell:  cycle.DwellSeg{Angle: 30},
		Return:    cycle.MotionSeg{Angle: 210, Type: "cycloid"},
		NearDwell: cycle.DwellSeg{Angle: 0},
	}
	if err := c2.Validate(); err != nil {
		t.Fatalf("循环档校验失败: %v", err)
	}
	res2, err := cycle.Build(&c2, 6, 1)
	if err != nil {
		t.Fatal(err)
	}
	last := res2.Report.Junctions[len(res2.Report.Junctions)-1]
	if math.Abs(last.Left.At.S) > 1e-9 || math.Abs(last.Right.At.S) > 1e-9 {
		t.Errorf("省略近休止后闭环接头两侧位移都应为 0，得到 %v -> %v",
			last.Left.At.S, last.Right.At.S)
	}
}

// 停歇段 s 保持 0 或 h，v、a、j 全为 0。
func TestDwellDerivativesZero(t *testing.T) {
	c := fullCycle(t, "cycloid", "cosine", 60, 90)
	res, err := cycle.Build(&c, 6, 1)
	if err != nil {
		t.Fatal(err)
	}
	// 找每段的角度区间，核查段内采样点。
	for _, pt := range res.Points {
		var inFar, inNear bool
		for _, sg := range res.Segments {
			if sg.Kind == "dwell_far" {
				inFar = pt.Theta > sg.Start+1e-9 && pt.Theta < sg.End-1e-9
			}
			if sg.Kind == "dwell_near" {
				inNear = pt.Theta > sg.Start+1e-9 && pt.Theta < sg.End-1e-9
			}
		}
		switch {
		case inFar:
			if math.Abs(pt.S-c.H) > 1e-9 || pt.V != 0 || pt.A != 0 || pt.J != 0 {
				t.Errorf("远休止点 θ=%v 状态错误: %+v", pt.Theta, pt)
			}
		case inNear:
			if math.Abs(pt.S) > 1e-9 || pt.V != 0 || pt.A != 0 || pt.J != 0 {
				t.Errorf("近休止点 θ=%v 状态错误: %+v", pt.Theta, pt)
			}
		}
	}
}

// 等加速接停歇：位移、速度连续；加速度不连续必须如实报告，不许粉饰。
func TestParabolicDwellAccelerationJumpReported(t *testing.T) {
	c := fullCycle(t, "parabolic", "parabolic", 60, 90)
	res, err := cycle.Build(&c, 6, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range res.Report.Junctions {
		if !j.SContinuous || !j.VContinuous {
			t.Errorf("接头 θ=%v 位移、速度必须连续", j.Theta)
		}
	}
	// 升程->远休止 与 远休止->回程 这两个接头，等加速端加速度不为 0。
	j0 := res.Report.Junctions[0] // 升程 -> 远休止，θ=120
	if j0.AContinuous {
		t.Errorf("等加速升程接远休止时加速度不应连续（端值 -4hk²），接头: %+v", j0)
	}
	if j0.Left.At.A == 0 {
		t.Errorf("等加速升程终点加速度不应为 0")
	}
}

func TestCycleValidationRejects(t *testing.T) {
	good := fullCycle(t, "cycloid", "cycloid", 70, 80)

	badAngle := good
	badAngle.FarDwell.Angle = 71 // 和不再等于 360
	if err := badAngle.Validate(); err == nil {
		t.Error("四段角度之和不等于一周应拒绝")
	}

	badType := good
	badType.Rise.Type = "unknown_law"
	if err := badType.Validate(); err == nil {
		t.Error("未知规律类型应拒绝")
	}

	badH := good
	badH.H = 0
	if err := badH.Validate(); err == nil {
		t.Error("h 非正应拒绝")
	}

	badNegDwell := good
	badNegDwell.FarDwell.Angle = -1
	badNegDwell.NearDwell.Angle = 81
	if err := badNegDwell.Validate(); err == nil {
		t.Error("负停歇角应拒绝")
	}

	badUnit := good
	badUnit.Unit = "grad"
	if err := badUnit.Validate(); err == nil {
		t.Error("未知角度单位应拒绝")
	}

	if err := good.Validate(); err != nil {
		t.Errorf("合法循环档被拒: %v", err)
	}
	if _, err := cycle.Build(&good, 0, 1); err == nil {
		t.Error("omega 非正应拒绝")
	}
}

// 弧度制一周为 2π。
func TestRadianFullTurn(t *testing.T) {
	c := cycle.Cycle{
		Name: "rad", Unit: "radian", H: 3,
		Rise:      cycle.MotionSeg{Angle: math.Pi / 2, Type: "cycloid"},
		FarDwell:  cycle.DwellSeg{Angle: math.Pi / 4},
		Return:    cycle.MotionSeg{Angle: math.Pi / 2, Type: "cycloid"},
		NearDwell: cycle.DwellSeg{Angle: 2 * math.Pi - math.Pi/2 - math.Pi/4 - math.Pi/2},
	}
	res, err := cycle.Build(&c, 1, 0.01)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.FullTurn-2*math.Pi) > 1e-12 {
		t.Errorf("弧度制一周应为 2π，得到 %v", res.FullTurn)
	}
	last := res.Points[len(res.Points)-1]
	if math.Abs(last.Theta-2*math.Pi) > 1e-9 || math.Abs(last.S) > 1e-9 {
		t.Errorf("弧度制闭环错误: θ=%v s=%v", last.Theta, last.S)
	}
}
