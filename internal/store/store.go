// Package store 把运动规律配方与循环档以本地 JSON 文件保存。
package store

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"camkin/internal/cycle"
	"camkin/internal/laws"
)

// Recipe 是一条规律配方：名字、类型、h、beta。
type Recipe struct {
	Name string  `json:"name"`
	Type string  `json:"type"`
	H    float64 `json:"h"`
	Beta float64 `json:"beta"` // 度；求曲线时若指定 radian 会先换算并再次校验
}

// Validate 拒绝缺项、beta 越界、未知类型，并说明原因。
// beta 以度登记，必须为正且小于一周（360 度）。
func (r *Recipe) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("配方缺少 name")
	}
	if !validFileName(r.Name) {
		return fmt.Errorf("配方名只能含字母、数字、下划线、短横与点，收到 %q", r.Name)
	}
	if _, err := laws.Get(r.Type); err != nil {
		return err
	}
	if !(r.H > 0) {
		return fmt.Errorf("配方 %q 的 h 必须为正，收到 %v", r.Name, r.H)
	}
	if !(r.Beta > 0) {
		return fmt.Errorf("配方 %q 的 beta 必须为正，收到 %v", r.Name, r.Beta)
	}
	if r.Beta >= 360 {
		return fmt.Errorf("配方 %q 的 beta(%v 度) 必须小于一周 360 度", r.Name, r.Beta)
	}
	if math.IsNaN(r.H) || math.IsInf(r.H, 0) || math.IsNaN(r.Beta) || math.IsInf(r.Beta, 0) {
		return fmt.Errorf("配方 %q 的 h、beta 必须是有限数值", r.Name)
	}
	return nil
}

// Store 是配方与循环档的本地文件仓库。
type Store struct {
	root string
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9_.\-一-龥]+$`)

func validFileName(name string) bool {
	return nameRe.MatchString(name) && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

// Open 打开（必要时创建）数据目录。
func Open(root string) (*Store, error) {
	for _, sub := range []string{"recipes", "cycles"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录失败: %w", err)
		}
	}
	return &Store{root: root}, nil
}

// SeedIfEmpty 在配方目录为空时载入启动默认配方：一条摆线升程。
func (s *Store) SeedIfEmpty() (bool, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "recipes"))
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			return false, nil
		}
	}
	seed := Recipe{Name: "default_cycloid", Type: laws.Cycloid, H: 10, Beta: 120}
	if err := s.PutRecipe(seed, true); err != nil {
		return false, err
	}
	return true, nil
}

// ---- 配方 ----

// PutRecipe 保存配方；overwrite 为 false 时重名拒绝。
func (s *Store) PutRecipe(r Recipe, overwrite bool) error {
	if err := r.Validate(); err != nil {
		return err
	}
	path := s.recipePath(r.Name)
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("配方 %q 已存在", r.Name)
		}
	}
	return writeJSON(path, r)
}

// GetRecipe 读取一条配方。
func (s *Store) GetRecipe(name string) (Recipe, error) {
	if !validFileName(name) {
		return Recipe{}, fmt.Errorf("非法配方名 %q", name)
	}
	var r Recipe
	if err := readJSON(s.recipePath(name), &r); err != nil {
		if os.IsNotExist(err) {
			return Recipe{}, fmt.Errorf("配方 %q 不存在", name)
		}
		return Recipe{}, err
	}
	return r, nil
}

// DeleteRecipe 删除一条配方。
func (s *Store) DeleteRecipe(name string) error {
	if !validFileName(name) {
		return fmt.Errorf("非法配方名 %q", name)
	}
	if err := os.Remove(s.recipePath(name)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("配方 %q 不存在", name)
		}
		return err
	}
	return nil
}

// RecipeInfo 是配方列表项。
type RecipeInfo struct {
	Recipe
	BetaUnit string `json:"beta_unit"`
}

// ListRecipes 列出全部配方（含类型与参数）。
func (s *Store) ListRecipes() ([]RecipeInfo, error) {
	names, err := listJSON(filepath.Join(s.root, "recipes"))
	if err != nil {
		return nil, err
	}
	out := make([]RecipeInfo, 0, len(names))
	for _, n := range names {
		r, err := s.GetRecipe(n)
		if err != nil {
			return nil, err
		}
		out = append(out, RecipeInfo{Recipe: r, BetaUnit: "degree"})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) recipePath(name string) string {
	return filepath.Join(s.root, "recipes", name+".json")
}

// ---- 循环档 ----

// PutCycle 保存循环档，保存前执行完整校验。
func (s *Store) PutCycle(c cycle.Cycle, overwrite bool) error {
	if !validFileName(c.Name) {
		return fmt.Errorf("循环档名只能含字母、数字、下划线、短横与点，收到 %q", c.Name)
	}
	if err := c.Validate(); err != nil {
		return err
	}
	path := s.cyclePath(c.Name)
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("循环档 %q 已存在", c.Name)
		}
	}
	return writeJSON(path, c)
}

// GetCycle 读取一条循环档。
func (s *Store) GetCycle(name string) (cycle.Cycle, error) {
	if !validFileName(name) {
		return cycle.Cycle{}, fmt.Errorf("非法循环档名 %q", name)
	}
	var c cycle.Cycle
	if err := readJSON(s.cyclePath(name), &c); err != nil {
		if os.IsNotExist(err) {
			return cycle.Cycle{}, fmt.Errorf("循环档 %q 不存在", name)
		}
		return cycle.Cycle{}, err
	}
	return c, nil
}

// DeleteCycle 删除一条循环档。
func (s *Store) DeleteCycle(name string) error {
	if !validFileName(name) {
		return fmt.Errorf("非法循环档名 %q", name)
	}
	if err := os.Remove(s.cyclePath(name)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("循环档 %q 不存在", name)
		}
		return err
	}
	return nil
}

// ListCycles 列出全部循环档。
func (s *Store) ListCycles() ([]cycle.Cycle, error) {
	names, err := listJSON(filepath.Join(s.root, "cycles"))
	if err != nil {
		return nil, err
	}
	out := make([]cycle.Cycle, 0, len(names))
	for _, n := range names {
		c, err := s.GetCycle(n)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) cyclePath(name string) string {
	return filepath.Join(s.root, "cycles", name+".json")
}

// ---- 通用 ----

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("解析 %s 失败: %w", filepath.Base(path), err)
	}
	return nil
}

func listJSON(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	return names, nil
}
