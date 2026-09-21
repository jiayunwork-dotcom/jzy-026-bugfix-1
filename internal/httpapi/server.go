// Package httpapi 对外提供 HTTP：规律目录、配方与循环档管理、升程曲线与一周循环曲线。
package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	"camkin/internal/cycle"
	"camkin/internal/kinematics"
	"camkin/internal/laws"
	"camkin/internal/store"
)

// Server 持有档仓库。
type Server struct {
	store *store.Store
	mux   *http.ServeMux
}

// New 构建路由。
func New(st *store.Store) *Server {
	s := &Server{store: st, mux: http.NewServeMux()}
	s.routes()
	return s
}

// ServeHTTP 实现 http.Handler。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	s.mux.HandleFunc("GET /api/laws", s.listLaws)

	s.mux.HandleFunc("GET /api/recipes", s.listRecipes)
	s.mux.HandleFunc("POST /api/recipes", s.createRecipe)
	s.mux.HandleFunc("GET /api/recipes/{name}", s.getRecipe)
	s.mux.HandleFunc("PUT /api/recipes/{name}", s.putRecipe)
	s.mux.HandleFunc("DELETE /api/recipes/{name}", s.deleteRecipe)
	s.mux.HandleFunc("GET /api/recipes/{name}/rise", s.recipeRise)

	// 不登记配方、直接点名规律求升程曲线。
	s.mux.HandleFunc("POST /api/rise", s.adhocRise)

	s.mux.HandleFunc("GET /api/cycles", s.listCycles)
	s.mux.HandleFunc("POST /api/cycles", s.createCycle)
	s.mux.HandleFunc("GET /api/cycles/{name}", s.getCycle)
	s.mux.HandleFunc("PUT /api/cycles/{name}", s.putCycle)
	s.mux.HandleFunc("DELETE /api/cycles/{name}", s.deleteCycle)
	s.mux.HandleFunc("GET /api/cycles/{name}/curve", s.cycleCurve)
}

func (s *Server) listLaws(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"laws":          laws.Catalog(),
		"angle_units":   []string{kinematics.Degree, kinematics.Radian},
		"dimensionless": "T=theta/beta in [0,1]; v/a/j derived analytically, restored by h*(omega/beta)^k",
	})
}

// ---- 配方 ----

func (s *Server) listRecipes(w http.ResponseWriter, _ *http.Request) {
	rs, err := s.store.ListRecipes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recipes": rs})
}

func (s *Server) createRecipe(w http.ResponseWriter, r *http.Request) {
	var rec store.Recipe
	if err := decodeBody(r, &rec); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.PutRecipe(rec, false); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Location", "/api/recipes/"+rec.Name)
	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) putRecipe(w http.ResponseWriter, r *http.Request) {
	var rec store.Recipe
	if err := decodeBody(r, &rec); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rec.Name = r.PathValue("name")
	if err := s.store.PutRecipe(rec, true); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) getRecipe(w http.ResponseWriter, r *http.Request) {
	rec, err := s.store.GetRecipe(r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) deleteRecipe(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteRecipe(r.PathValue("name")); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// riseRequest 是配方升程曲线的查询参数。
type riseQuery struct {
	Omega float64
	Unit  string
	N     int
}

func parseRiseQuery(r *http.Request) (riseQuery, error) {
	q := riseQuery{Unit: kinematics.Degree, N: 240}
	v := r.URL.Query()
	if raw := v.Get("unit"); raw != "" {
		u := strings.ToLower(raw)
		switch u {
		case "degree", "deg", "degrees":
			q.Unit = kinematics.Degree
		case "radian", "rad", "radians":
			q.Unit = kinematics.Radian
		default:
			return q, fmt.Errorf("未知角度单位 %q，仅支持 degree / radian", raw)
		}
	}
	omega, err := strconv.ParseFloat(v.Get("omega"), 64)
	if err != nil {
		return q, fmt.Errorf("缺少或非法的 query 参数 omega（必须为正）")
	}
	q.Omega = omega
	if raw := v.Get("n"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return q, fmt.Errorf("query 参数 n 必须为正整数，收到 %q", raw)
		}
		q.N = n
	}
	return q, nil
}

func (s *Server) recipeRise(w http.ResponseWriter, r *http.Request) {
	rec, err := s.store.GetRecipe(r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	q, err := parseRiseQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	law, err := laws.Get(rec.Type) // 未知类型在求曲线前直接拒绝
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	p := kinematics.Params{H: rec.H, Beta: rec.Beta, Omega: q.Omega, Unit: kinematics.Degree}
	if q.Unit == kinematics.Radian {
		p.Beta = rec.Beta * math.Pi / 180 // 配方 beta 以度登记
		p.Unit = kinematics.Radian
	}
	res, err := kinematics.RiseCurve(law, p, q.N)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// adhocRiseBody 是直接点名规律求升程曲线的请求体。
type adhocRiseBody struct {
	Type  string  `json:"type"`
	H     float64 `json:"h"`
	Beta  float64 `json:"beta"`
	Omega float64 `json:"omega"`
	Unit  string  `json:"unit"`
	N     int     `json:"n"`
}

func (s *Server) adhocRise(w http.ResponseWriter, r *http.Request) {
	var b adhocRiseBody
	if err := decodeBody(r, &b); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	law, err := laws.Get(b.Type) // 未知规律名在求曲线前直接拒绝
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	unit := b.Unit
	if unit == "" {
		unit = kinematics.Degree
	}
	if b.N == 0 {
		b.N = 240
	}
	p := kinematics.Params{H: b.H, Beta: b.Beta, Omega: b.Omega, Unit: unit}
	res, err := kinematics.RiseCurve(law, p, b.N)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ---- 循环档 ----

func (s *Server) listCycles(w http.ResponseWriter, _ *http.Request) {
	cs, err := s.store.ListCycles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cycles": cs})
}

func (s *Server) createCycle(w http.ResponseWriter, r *http.Request) {
	c, err := decodeCycle(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.PutCycle(c, false); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Location", "/api/cycles/"+c.Name)
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) putCycle(w http.ResponseWriter, r *http.Request) {
	c, err := decodeCycle(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	c.Name = r.PathValue("name")
	if err := s.store.PutCycle(c, true); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func decodeCycle(r *http.Request) (cycle.Cycle, error) {
	var c cycle.Cycle
	if err := decodeBody(r, &c); err != nil {
		return c, err
	}
	return c, c.Validate()
}

func (s *Server) getCycle(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.GetCycle(r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) deleteCycle(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteCycle(r.PathValue("name")); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) cycleCurve(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.GetCycle(r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	v := r.URL.Query()
	omega, err := strconv.ParseFloat(v.Get("omega"), 64)
	if err != nil || !(omega > 0) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("缺少或非法的 query 参数 omega（必须为正）"))
		return
	}
	step := kinematics.FullTurn(c.Unit) / 720
	if raw := v.Get("step"); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || !(parsed > 0) {
			writeError(w, http.StatusBadRequest, fmt.Errorf("query 参数 step 必须为正，收到 %q", raw))
			return
		}
		step = parsed
	}
	res, err := cycle.Build(&c, omega, step)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ---- 工具 ----

func decodeBody(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("请求体不是合法 JSON: %w", err)
	}
	return nil
}

func writeError(w http.ResponseWriter, code int, err error) {
	log.Printf("http %d: %v", code, err)
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func writeStoreError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "不存在") || strings.Contains(err.Error(), "非法") {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		log.Printf("写响应失败: %v", err)
	}
}
