package model

import (
	"testing"
	"time"
)

func TestRankedGroupsAndModels(t *testing.T) {
	// 测试分组缓存与拷贝独立性
	rankedGroupsCacheMu.Lock()
	oldGCache := rankedGroupsCache
	oldGExp := rankedGroupsCacheExp
	rankedGroupsCache = []string{"Gemini组", "cc反重力按量"}
	rankedGroupsCacheExp = time.Now().Add(2 * time.Hour)
	rankedGroupsCacheMu.Unlock()

	defer func() {
		rankedGroupsCacheMu.Lock()
		rankedGroupsCache = oldGCache
		rankedGroupsCacheExp = oldGExp
		rankedGroupsCacheMu.Unlock()
	}()

	groups := GetRankedGroups()
	if len(groups) != 2 || groups[0] != "Gemini组" {
		t.Fatalf("unexpected cached groups: %v", groups)
	}

	groups[0] = "mutated"
	groupsFresh := GetRankedGroups()
	if groupsFresh[0] != "Gemini组" {
		t.Fatalf("cache was mutated externally")
	}

	// 测试模型缓存与拷贝独立性
	rankedModelsCacheMu.Lock()
	oldMCache := rankedModelsCache
	oldMExp := rankedModelsCacheExp
	rankedModelsCache = []string{"gemini-3.8-flash", "claude-opus-4-6"}
	rankedModelsCacheExp = time.Now().Add(2 * time.Hour)
	rankedModelsCacheMu.Unlock()

	defer func() {
		rankedModelsCacheMu.Lock()
		rankedModelsCache = oldMCache
		rankedModelsCacheExp = oldMExp
		rankedModelsCacheMu.Unlock()
	}()

	models := GetRankedModels()
	if len(models) != 2 || models[0] != "gemini-3.8-flash" {
		t.Fatalf("unexpected cached models: %v", models)
	}

	models[0] = "mutated"
	modelsFresh := GetRankedModels()
	if modelsFresh[0] != "gemini-3.8-flash" {
		t.Fatalf("cache was mutated externally")
	}
}
