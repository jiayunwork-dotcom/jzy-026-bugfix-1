package store_test

import (
	"testing"

	"camkin/internal/cycle"
	"camkin/internal/laws"
	"camkin/internal/store"
)

func TestSeedOnEmpty(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := st.SeedIfEmpty()
	if err != nil || !seeded {
		t.Fatalf("空目录应播种默认配方，seeded=%v err=%v", seeded, err)
	}
	rec, err := st.GetRecipe("default_cycloid")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Type != laws.Cycloid {
		t.Errorf("默认配方应为摆线，得到 %q", rec.Type)
	}
	if !(rec.H > 0 && rec.Beta > 0 && rec.Beta < 360) {
		t.Errorf("默认配方参数非法: %+v", rec)
	}
	// 再次启动不应重复播种。
	seeded2, err := st.SeedIfEmpty()
	if err != nil || seeded2 {
		t.Errorf("已有配方时不应播种，seeded=%v err=%v", seeded2, err)
	}
}

func TestRecipeValidation(t *testing.T) {
	st, _ := store.Open(t.TempDir())
	good := store.Recipe{Name: "r1", Type: laws.Cycloid, H: 5, Beta: 90}
	if err := st.PutRecipe(good, false); err != nil {
		t.Fatalf("合法配方被拒: %v", err)
	}
	if err := st.PutRecipe(good, false); err == nil {
		t.Error("重名配方应拒绝")
	}
	bad := []store.Recipe{
		{Name: "", Type: laws.Cosine, H: 1, Beta: 90},            // 缺名
		{Name: "a/b", Type: laws.Cosine, H: 1, Beta: 90},         // 非法名
		{Name: "x", Type: "no_such_law", H: 1, Beta: 90},         // 未知类型
		{Name: "x", Type: laws.Cosine, H: 0, Beta: 90},           // h
		{Name: "x", Type: laws.Cosine, H: 1, Beta: 0},            // beta
		{Name: "x", Type: laws.Cosine, H: 1, Beta: 360},          // beta 越界
		{Name: "x", Type: laws.Cosine, H: 1, Beta: 400},          // beta 越界
	}
	for i, r := range bad {
		if err := st.PutRecipe(r, true); err == nil {
			t.Errorf("非法配方 %d 应被拒绝: %+v", i, r)
		}
	}
	infos, err := st.ListRecipes()
	if err != nil || len(infos) != 1 {
		t.Fatalf("列表应只有 1 条合法配方，得到 %d, err=%v", len(infos), err)
	}
	if infos[0].Type != laws.Cycloid || infos[0].BetaUnit != "degree" {
		t.Errorf("列表项应给出类型与参数单位: %+v", infos[0])
	}
}

func TestCyclePersistence(t *testing.T) {
	st, _ := store.Open(t.TempDir())
	c := cycle.Cycle{
		Name: "c1", Unit: "degree", H: 10,
		Rise:      cycle.MotionSeg{Angle: 120, Type: "cycloid"},
		FarDwell:  cycle.DwellSeg{Angle: 60},
		Return:    cycle.MotionSeg{Angle: 120, Type: "cosine"},
		NearDwell: cycle.DwellSeg{Angle: 60},
	}
	if err := st.PutCycle(c, false); err != nil {
		t.Fatalf("合法循环档被拒: %v", err)
	}
	got, err := st.GetCycle("c1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Rise.Type != "cycloid" || got.Return.Type != "cosine" {
		t.Errorf("循环档读写类型丢失: %+v", got)
	}
	bad := c
	bad.Name = "bad"
	bad.FarDwell.Angle = 61 // 和不为 360
	if err := st.PutCycle(bad, false); err == nil {
		t.Error("角度不闭合的循环档应拒绝")
	}
}
