package httpapi_test

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"camkin/internal/cycle"
	"camkin/internal/httpapi"
	"camkin/internal/laws"
	"camkin/internal/store"
)

func newServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SeedIfEmpty(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(httpapi.New(st))
	t.Cleanup(srv.Close)
	return srv, st
}

func getJSON(t *testing.T, url string, wantCode int) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantCode {
		t.Fatalf("GET %s 状态码 %d, 期望 %d", url, resp.StatusCode, wantCode)
	}
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func postJSON(t *testing.T, method, url, body string, wantCode int) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantCode {
		t.Fatalf("%s %s 状态码 %d, 期望 %d; body=%s", method, url, resp.StatusCode, wantCode, body)
	}
	var m map[string]any
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestLawsCatalog(t *testing.T) {
	srv, _ := newServer(t)
	m := getJSON(t, srv.URL+"/api/laws", http.StatusOK)
	arr, ok := m["laws"].([]any)
	if !ok || len(arr) != 3 {
		t.Fatalf("规律目录应有 3 条，得到 %v", m["laws"])
	}
}

func TestUnknownLawRejectedOverHTTP(t *testing.T) {
	srv, _ := newServer(t)
	m := postJSON(t, "POST", srv.URL+"/api/rise",
		`{"type":"ghost","h":10,"beta":90,"omega":6,"unit":"degree"}`, http.StatusBadRequest)
	if !strings.Contains(m["error"].(string), "未知规律") {
		t.Errorf("错误信息应说明未知规律，得到 %v", m["error"])
	}
	// 未知类型登记配方同样被拒。
	m = postJSON(t, "POST", srv.URL+"/api/recipes",
		`{"name":"bad","type":"ghost","h":10,"beta":90}`, http.StatusBadRequest)
	if !strings.Contains(m["error"].(string), "未知规律") {
		t.Errorf("登记未知类型应拒绝，得到 %v", m["error"])
	}
}

func TestRecipeRiseEndpoint(t *testing.T) {
	srv, _ := newServer(t)
	url := srv.URL + "/api/recipes/default_cycloid/rise?omega=6&n=360&unit=degree"
	m := getJSON(t, url, http.StatusOK)
	if m["angle_unit"] != "degree" {
		t.Errorf("结果应写明角度单位，得到 %v", m["angle_unit"])
	}
	start := m["start"].(map[string]any)
	end := m["end"].(map[string]any)
	if start["s"].(float64) != 0 {
		t.Errorf("起点 s=%v 期望 0", start["s"])
	}
	if math.Abs(end["s"].(float64)-10) > 1e-9 {
		t.Errorf("终点 s=%v 期望 h=10", end["s"])
	}
	if start["a"].(float64) != 0 || end["a"].(float64) != 0 {
		t.Errorf("摆线两端 a 应为 0，得到 %v, %v", start["a"], end["a"])
	}
	peaks := m["analytic_peaks"].(map[string]any)
	k := 6.0 / 120
	wantA := 2 * math.Pi * 10 * k * k
	if math.Abs(peaks["a_max"].(float64)-wantA) > 1e-9 {
		t.Errorf("a_max=%v 闭式=%v", peaks["a_max"], wantA)
	}
	// 网格首末点也必须与闭式一致。
	pts := m["points"].([]any)
	first := pts[0].(map[string]any)
	last := pts[len(pts)-1].(map[string]any)
	if first["s"].(float64) != 0 || math.Abs(last["s"].(float64)-10) > 1e-9 {
		t.Errorf("网格端点 s 与闭式不符: %v %v", first["s"], last["s"])
	}
}

func TestScalingOverHTTP(t *testing.T) {
	srv, _ := newServer(t)
	body := func(h float64) string {
		b, _ := json.Marshal(map[string]any{
			"type": "cycloid", "h": h, "beta": 120, "omega": 6, "unit": "degree", "n": 100,
		})
		return string(b)
	}
	r1 := postJSON(t, "POST", srv.URL+"/api/rise", body(5), http.StatusOK)
	r2 := postJSON(t, "POST", srv.URL+"/api/rise", body(15), http.StatusOK)
	p1 := r1["points"].([]any)
	p2 := r2["points"].([]any)
	if len(p1) != len(p2) {
		t.Fatal("网格长度不一致")
	}
	for i := range p1 {
		q1 := p1[i].(map[string]any)
		q2 := p2[i].(map[string]any)
		for _, key := range []string{"s", "v", "a", "j"} {
			got := q2[key].(float64)
			want := 3 * q1[key].(float64)
			if math.Abs(got-want) > 1e-7*math.Max(1, math.Abs(want)) {
				t.Errorf("h 放大 3 倍，%s 应同比例: %v -> %v（期望 %v）", key, q1[key], got, want)
			}
		}
	}
}

func TestCycleCurveHTTP(t *testing.T) {
	srv, _ := newServer(t)
	body, _ := json.Marshal(cycle.Cycle{
		Name: "full", Unit: "degree", H: 10,
		Rise:      cycle.MotionSeg{Angle: 120, Type: laws.Parabolic},
		FarDwell:  cycle.DwellSeg{Angle: 60},
		Return:    cycle.MotionSeg{Angle: 90, Type: laws.Parabolic},
		NearDwell: cycle.DwellSeg{Angle: 90},
	})
	postJSON(t, "POST", srv.URL+"/api/cycles", string(body), http.StatusCreated)
	m := getJSON(t, srv.URL+"/api/cycles/full/curve?omega=6&step=1", http.StatusOK)
	if m["angle_unit"] != "degree" {
		t.Errorf("应写明角度单位: %v", m["angle_unit"])
	}
	rep := m["continuity"].(map[string]any)
	if rep["displacement_continuous"] != true {
		t.Errorf("整周位移必须连续: %v", rep)
	}
	pts := m["points"].([]any)
	last := pts[len(pts)-1].(map[string]any)
	if math.Abs(last["theta"].(float64)-360) > 1e-9 || math.Abs(last["s"].(float64)) > 1e-9 {
		t.Errorf("一周终点应为 θ=360,s=0，得到 %v %v", last["theta"], last["s"])
	}

	// 停歇角为 0 的循环：不得把相邻段接错。
	body0, _ := json.Marshal(cycle.Cycle{
		Name: "zero", Unit: "degree", H: 10,
		Rise:      cycle.MotionSeg{Angle: 120, Type: "cycloid"},
		FarDwell:  cycle.DwellSeg{Angle: 0},
		Return:    cycle.MotionSeg{Angle: 240, Type: "cycloid"},
		NearDwell: cycle.DwellSeg{Angle: 0},
	})
	postJSON(t, "POST", srv.URL+"/api/cycles", string(body0), http.StatusCreated)
	m0 := getJSON(t, srv.URL+"/api/cycles/zero/curve?omega=6&step=2", http.StatusOK)
	rep0 := m0["continuity"].(map[string]any)
	if rep0["displacement_continuous"] != true || rep0["velocity_continuous"] != true {
		t.Errorf("省略停歇的摆线循环位移、速度都应连续: %v", rep0)
	}

	// 缺 omega 必须报错。
	getJSON(t, srv.URL+"/api/cycles/full/curve", http.StatusBadRequest)
}

func TestRecipeCRUDAndRejects(t *testing.T) {
	srv, _ := newServer(t)
	// 缺项（h 缺失）拒绝。
	postJSON(t, "POST", srv.URL+"/api/recipes",
		`{"name":"r","type":"cosine","beta":90}`, http.StatusBadRequest)
	// beta 越界拒绝。
	postJSON(t, "POST", srv.URL+"/api/recipes",
		`{"name":"r","type":"cosine","h":1,"beta":360}`, http.StatusBadRequest)
	// 合法创建。
	postJSON(t, "POST", srv.URL+"/api/recipes",
		`{"name":"r","type":"cosine","h":2,"beta":60}`, http.StatusCreated)
	getJSON(t, srv.URL+"/api/recipes/r", http.StatusOK)
	getJSON(t, srv.URL+"/api/recipes/missing", http.StatusNotFound)
	// 列表给出类型与参数。
	m := getJSON(t, srv.URL+"/api/recipes", http.StatusOK)
	names := m["recipes"].([]any)
	if len(names) < 2 {
		t.Errorf("列表至少含默认配方与新建配方，得到 %d", len(names))
	}
	// 删除后 404。
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/recipes/r", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("删除状态码 %d", resp.StatusCode)
	}
	getJSON(t, srv.URL+"/api/recipes/r", http.StatusNotFound)
}

func TestInvalidParamsRejected(t *testing.T) {
	srv, _ := newServer(t)
	cases := []string{
		`{"type":"cycloid","h":-1,"beta":90,"omega":6}`,
		`{"type":"cycloid","h":1,"beta":900,"omega":6}`,
		`{"type":"cycloid","h":1,"beta":90,"omega":-6}`,
		`{"type":"cycloid","h":1,"beta":90,"omega":6,"unit":"grad"}`,
	}
	for _, c := range cases {
		req, _ := http.NewRequest("POST", srv.URL+"/api/rise", bytes.NewBufferString(c))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("非法参数应 400（%s），得到 %d", c, resp.StatusCode)
		}
	}
}
