// Package cycle 把升程段、远休止、回程段、近休止拼成完整一周的循环曲线，
// 并在每个拼接角核对该连续的量（位移必连续，速度在端点为 0 也应连续；
// 加速度、跃度按规律性质如实报告，不做粉饰）。
package cycle

import (
	"fmt"
	"math"

	"camkin/internal/kinematics"
	"camkin/internal/laws"
)

// MotionSeg 是一段有运动规律的段（升程或回程）。
type MotionSeg struct {
	Angle float64 `json:"angle"` // 段角（与 cycle 角度单位一致）
	Type  string  `json:"type"`  // 规律类型：cosine / cycloid / parabolic
}

// DwellSeg 是停歇段。Angle 为 0 表示该段省略。
type DwellSeg struct {
	Angle float64 `json:"angle"`
}

// Cycle 是一条完整循环档：四段角度与各运动段类型。
type Cycle struct {
	Name      string   `json:"name"`
	Unit      string   `json:"unit"`
	H         float64  `json:"h"`
	Rise      MotionSeg `json:"rise"`
	FarDwell  DwellSeg  `json:"far_dwell"`
	Return    MotionSeg `json:"return"`
	NearDwell DwellSeg  `json:"near_dwell"`
}

// Validate 校验循环档：缺项、角度越界、未知类型一律拒绝并说明原因。
func (c *Cycle) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("循环档缺少 name")
	}
	if !kinematics.ValidUnit(c.Unit) {
		return fmt.Errorf("角度单位必须是 %q 或 %q，收到 %q",
			kinematics.Degree, kinematics.Radian, c.Unit)
	}
	if !(c.H > 0) {
		return fmt.Errorf("升程 h 必须为正，收到 %v", c.H)
	}
	full := kinematics.FullTurn(c.Unit)
	checkMotion := func(seg MotionSeg, label string) error {
		if !(seg.Angle > 0) {
			return fmt.Errorf("%s段角必须为正，收到 %v", label, seg.Angle)
		}
		if seg.Angle >= full {
			return fmt.Errorf("%s段角 %v 必须小于一周 %v", label, seg.Angle, full)
		}
		if _, err := laws.Get(seg.Type); err != nil {
			return fmt.Errorf("%s段：%w", label, err)
		}
		return nil
	}
	if err := checkMotion(c.Rise, "升程"); err != nil {
		return err
	}
	if err := checkMotion(c.Return, "回程"); err != nil {
		return err
	}
	if c.FarDwell.Angle < 0 {
		return fmt.Errorf("远休止角不能为负，收到 %v", c.FarDwell.Angle)
	}
	if c.NearDwell.Angle < 0 {
		return fmt.Errorf("近休止角不能为负，收到 %v", c.NearDwell.Angle)
	}
	total := c.Rise.Angle + c.FarDwell.Angle + c.Return.Angle + c.NearDwell.Angle
	if math.Abs(total-full) > eps*math.Max(1, full) {
		return fmt.Errorf("四段角度之和 %v 不等于一周 %v（%s）", total, full, c.Unit)
	}
	return nil
}

const eps = 1e-9

// Side 是拼接角一侧的状态。
type Side struct {
	Segment string             `json:"segment"`
	At      kinematics.State   `json:"state"`
}

// Junction 是一个拼接角的核对结果。
type Junction struct {
	Theta float64 `json:"theta"`
	Left  Side    `json:"left"`
	Right Side    `json:"right"`
	// S 在所有拼接角都必须连续；V 因各规律端点速度为 0 也必须连续。
	SContinuous bool `json:"s_continuous"`
	VContinuous bool `json:"v_continuous"`
	// A、J 按规律性质可能不连续（如余弦/等加速接停歇时加速度本就不为 0），
	// 只如实报告，不视为错误。
	AContinuous bool `json:"a_continuous"`
	JContinuous bool `json:"j_continuous"`
	Note        string `json:"note,omitempty"`
}

// Report 是整周连续性核对报告。
type Report struct {
	Junctions            []Junction `json:"junctions"`
	DisplacementContinuous bool      `json:"displacement_continuous"`
	VelocityContinuous     bool      `json:"velocity_continuous"`
}

// Result 是一周曲线的构建结果。
type Result struct {
	Unit      string             `json:"angle_unit"`
	FullTurn  float64            `json:"full_turn"`
	Points    []kinematics.Point `json:"points"`
	Segments  []SegmentInfo      `json:"segments"`
	Report    Report             `json:"continuity"`
	Observed  kinematics.Peaks   `json:"observed_peaks"`
}

// SegmentInfo 描述参与拼接的一段（角度为 0 的停歇段已省略）。
type SegmentInfo struct {
	Name  string  `json:"name"`
	Kind  string  `json:"kind"` // rise / dwell_far / return / dwell_near
	Type  string  `json:"type,omitempty"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type segment struct {
	info SegmentInfo
	// start/end 返回该段两端的解析有量纲状态。
	start, end kinematics.State
	// at 求段内自起点起 localTheta 处的状态。
	at func(localTheta float64) kinematics.State
}

func dwellState(level float64) kinematics.State {
	return kinematics.State{S: level}
}

// Build 校验循环档并沿转角从 0 到一周采样；step 为期望采样间隔，
// 每段至少切 1 段，拼接角必为采样点。
func Build(c *Cycle, omega, step float64) (*Result, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if !(omega > 0) {
		return nil, fmt.Errorf("角速度 omega 必须为正，收到 %v", omega)
	}
	if !(step > 0) {
		return nil, fmt.Errorf("采样间隔 step 必须为正，收到 %v", step)
	}
	full := kinematics.FullTurn(c.Unit)

	riseLaw := mustLaw(c.Rise.Type)
	retLaw := mustLaw(c.Return.Type)
	riseP := kinematics.Params{H: c.H, Beta: c.Rise.Angle, Omega: omega, Unit: c.Unit}
	retP := kinematics.Params{H: c.H, Beta: c.Return.Angle, Omega: omega, Unit: c.Unit}

	riseStart, riseEnd := kinematics.EndStates(riseLaw, riseP)
	retStart, retEnd := kinematics.ReturnEndStates(retLaw, retP)

	var segs []segment
	segs = append(segs, segment{
		info:  SegmentInfo{Name: "升程", Kind: "rise", Type: c.Rise.Type},
		start: riseStart, end: riseEnd,
		at: func(local float64) kinematics.State { return kinematics.RiseStateAt(riseLaw, riseP, local) },
	})
	if c.FarDwell.Angle > 0 {
		segs = append(segs, segment{
			info:  SegmentInfo{Name: "远休止", Kind: "dwell_far"},
			start: dwellState(c.H), end: dwellState(c.H),
			at:   func(float64) kinematics.State { return dwellState(c.H) },
		})
	}
	segs = append(segs, segment{
		info:  SegmentInfo{Name: "回程", Kind: "return", Type: c.Return.Type},
		start: retStart, end: retEnd,
		at: func(local float64) kinematics.State { return kinematics.ReturnStateAt(retLaw, retP, local) },
	})
	if c.NearDwell.Angle > 0 {
		segs = append(segs, segment{
			info:  SegmentInfo{Name: "近休止", Kind: "dwell_near"},
			start: dwellState(0), end: dwellState(0),
			at:   func(float64) kinematics.State { return dwellState(0) },
		})
	}

	// 定各段起止角。
	theta := 0.0
	angles := []float64{c.Rise.Angle, c.FarDwell.Angle, c.Return.Angle, c.NearDwell.Angle}
	idx := 0
	for i := range segs {
		for angles[idx] == 0 {
			idx++
		}
		segs[i].info.Start = theta
		theta += angles[idx]
		// 抵消求和误差，末段直接钉到一周。
		if i == len(segs)-1 {
			theta = full
		}
		segs[i].info.End = theta
		idx++
	}

	// 采样：拼接角必落在网格上，段内均匀。
	points := []kinematics.Point{{Theta: 0, State: segs[0].start}}
	obs := kinematics.Peaks{JBounded: true}
	accumObs := func(st kinematics.State) {
		obs.V = math.Max(obs.V, math.Abs(st.V))
		obs.A = math.Max(obs.A, math.Abs(st.A))
		obs.J = math.Max(obs.J, math.Abs(st.J))
	}
	accumObs(segs[0].start)
	for _, sg := range segs {
		angle := sg.info.End - sg.info.Start
		n := int(math.Ceil(angle/step))
		if n < 1 {
			n = 1
		}
		for k := 1; k <= n; k++ {
			local := angle * float64(k) / float64(n)
			st := sg.at(local)
			if k == n {
				st = sg.end // 端点用解析端值，杜绝网格与闭式对不上
			}
			points = append(points, kinematics.Point{Theta: sg.info.Start + local, State: st})
			accumObs(st)
		}
	}
	points[len(points)-1].Theta = full

	// 接头核对（含一周闭环处最后一段末尾接第一段起点）。
	report := Report{Junctions: nil, DisplacementContinuous: true, VelocityContinuous: true}
	for i := range segs {
		left := segs[i]
		right := segs[(i+1)%len(segs)]
		var at float64
		if i == len(segs)-1 {
			at = full // 闭环接头
		} else {
			at = left.info.End
		}
		j := makeJunction(at,
			Side{Segment: left.info.Name, At: left.end},
			Side{Segment: right.info.Name, At: right.start})
		report.Junctions = append(report.Junctions, j)
		if !j.SContinuous {
			report.DisplacementContinuous = false
		}
		if !j.VContinuous {
			report.VelocityContinuous = false
		}
	}
	if !report.DisplacementContinuous {
		return nil, fmt.Errorf("拼接角出现位移跳变，详见连续性核对")
	}

	infos := make([]SegmentInfo, len(segs))
	for i, sg := range segs {
		infos[i] = sg.info
	}
	return &Result{
		Unit: c.Unit, FullTurn: full,
		Points: points, Segments: infos,
		Report: report, Observed: obs,
	}, nil
}

func makeJunction(theta float64, left, right Side) Junction {
	tol := func(x, y float64) bool {
		return math.Abs(x-y) <= eps*math.Max(1, math.Max(math.Abs(x), math.Abs(y)))
	}
	j := Junction{
		Theta: theta, Left: left, Right: right,
		SContinuous: tol(left.At.S, right.At.S),
		VContinuous: tol(left.At.V, right.At.V),
		AContinuous: tol(left.At.A, right.At.A),
		JContinuous: tol(left.At.J, right.At.J),
	}
	switch {
	case !j.SContinuous:
		j.Note = "位移不连续：不允许"
	case !j.AContinuous && (left.At.A != 0 || right.At.A != 0):
		j.Note = "位移、速度连续；加速度不连续是该规律接停歇/回程时的已知性质"
	}
	return j
}

func mustLaw(typ string) laws.Law {
	l, err := laws.Get(typ)
	if err != nil {
		panic(err) // Build 前已 Validate，正常路径不可达
	}
	return l
}
