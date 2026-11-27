package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func encodeTinyPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 30, G: 120, B: 220, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFoldersSortSearchAndPerUserFavorites(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	env := newTestEnv(t, func(media string) {
		pngBytes := encodeTinyPNG(t, 40, 30)
		mp4Bytes := buildMinimalMP4(15000, 320, 180, 64)
		webmBytes := buildMinimalWebM(22000)

		writeFixtureFile(t, filepath.Join(media, "Screenshots", "Astro", "alpha.png"), pngBytes, t0)
		writeFixtureFile(t, filepath.Join(media, "Screenshots", "Astro", "zeta.png"), append(append([]byte{}, pngBytes...), 0, 0, 0), t0.Add(1*time.Hour))
		writeFixtureFile(t, filepath.Join(media, "Screenshots", "Astro", "clip.mp4"), mp4Bytes, t0.Add(2*time.Hour))
		writeFixtureFile(t, filepath.Join(media, "Videos", "boss.webm"), webmBytes, t0.Add(3*time.Hour))
	})

	env.addUser("alice", "pw1")
	env.addUser("bob", "pw2")
	tokA := env.login("alice", "pw1", false)
	tokB := env.login("bob", "pw2", false)

	// 1. Virtual root folder.
	rootResp := env.req(http.MethodGet, "/api/folders/root", tokA, nil)
	var root struct {
		Folder      *folderJSON  `json:"folder"`
		Children    []folderJSON `json:"children"`
		Breadcrumbs []folderJSON `json:"breadcrumbs"`
	}
	_ = json.NewDecoder(rootResp.Body).Decode(&root)
	rootResp.Body.Close()
	if rootResp.StatusCode != 200 || root.Folder != nil || len(root.Breadcrumbs) != 0 || len(root.Children) != 1 {
		t.Fatalf("unexpected root folder response: %d %+v", rootResp.StatusCode, root)
	}

	mediaRootID := root.Children[0].ID
	subResp := env.req(http.MethodGet, "/api/folders/"+mediaRootID, tokA, nil)
	var mediaRoot folderResponse
	_ = json.NewDecoder(subResp.Body).Decode(&mediaRoot)
	subResp.Body.Close()
	if len(mediaRoot.Children) != 2 {
		t.Fatalf("expected 2 child folders (Screenshots, Videos), got %+v", mediaRoot.Children)
	}

	var screenshotsID string
	for _, c := range mediaRoot.Children {
		if c.Name == "Screenshots" {
			screenshotsID = c.ID
		}
	}
	scrResp := env.req(http.MethodGet, "/api/folders/"+screenshotsID, tokA, nil)
	var scr folderResponse
	_ = json.NewDecoder(scrResp.Body).Decode(&scr)
	scrResp.Body.Close()
	if len(scr.Children) != 1 || scr.Children[0].Name != "Astro" || scr.Children[0].FileCount != 3 || scr.Children[0].CoverItemID == nil {
		t.Fatalf("unexpected Screenshots folder: %+v", scr)
	}
	astroID := scr.Children[0].ID

	// 2. Folder items: sort by name asc, type filter, pagination.
	itemsResp := env.req(http.MethodGet, "/api/folders/"+astroID+"/items?sort=name&order=asc&page=1&pageSize=2", tokA, nil)
	var page1 listResponse
	_ = json.NewDecoder(itemsResp.Body).Decode(&page1)
	itemsResp.Body.Close()
	if itemsResp.StatusCode != 200 || page1.Total != 3 || len(page1.Items) != 2 || page1.Items[0].Name != "alpha.png" || page1.Items[1].Name != "clip.mp4" {
		t.Fatalf("unexpected sorted page1: %+v", page1)
	}
	if page1.Folder == nil || page1.Folder.ID != astroID {
		t.Fatalf("expected folder metadata on folder items, got %+v", page1.Folder)
	}

	// Type filter=video.
	vidResp := env.req(http.MethodGet, "/api/folders/"+astroID+"/items?type=video", tokA, nil)
	var vidList listResponse
	_ = json.NewDecoder(vidResp.Body).Decode(&vidList)
	vidResp.Body.Close()
	if vidList.Total != 1 || vidList.Items[0].Name != "clip.mp4" || vidList.Items[0].DurationMs == nil || *vidList.Items[0].DurationMs != 15000 {
		t.Fatalf("unexpected video filter result: %+v", vidList)
	}

	// Invalid sort -> 400.
	badSort := env.req(http.MethodGet, "/api/folders/"+astroID+"/items?sort=bogus", tokA, nil)
	badSort.Body.Close()
	if badSort.StatusCode != 400 {
		t.Fatalf("expected 400 for invalid sort, got %d", badSort.StatusCode)
	}

	// 3. Per-user favorites isolation.
	targetID := page1.Items[0].ID
	favPut := env.req(http.MethodPut, "/api/items/"+targetID+"/favorite", tokA, map[string]bool{"favorite": true})
	favPut.Body.Close()
	if favPut.StatusCode != 200 {
		t.Fatalf("expected 200 setting favorite, got %d", favPut.StatusCode)
	}

	// Alice sees 1 favorite; Bob sees 0 favorites.
	favA := env.req(http.MethodGet, "/api/favorites", tokA, nil)
	var listA listResponseNoFolder
	_ = json.NewDecoder(favA.Body).Decode(&listA)
	favA.Body.Close()
	if listA.Total != 1 || listA.Items[0].ID != targetID || !listA.Items[0].Favorite {
		t.Fatalf("alice favorites unexpected: %+v", listA)
	}

	favB := env.req(http.MethodGet, "/api/favorites", tokB, nil)
	var listB listResponseNoFolder
	_ = json.NewDecoder(favB.Body).Decode(&listB)
	favB.Body.Close()
	if listB.Total != 0 {
		t.Fatalf("bob should have 0 favorites, got %+v", listB)
	}

	itemForB := env.req(http.MethodGet, "/api/items/"+targetID, tokB, nil)
	var itB itemJSON
	_ = json.NewDecoder(itemForB.Body).Decode(&itB)
	itemForB.Body.Close()
	if itB.Favorite {
		t.Fatalf("expected item favorite=false for bob, got true")
	}

	// 4. Search: empty q -> 400; substring case-insensitive -> matches.
	emptyQ := env.req(http.MethodGet, "/api/search?q=", tokA, nil)
	emptyQ.Body.Close()
	if emptyQ.StatusCode != 400 {
		t.Fatalf("expected 400 for empty search, got %d", emptyQ.StatusCode)
	}
	qResp := env.req(http.MethodGet, "/api/search?q=BOSS", tokA, nil)
	var qRes listResponseNoFolder
	_ = json.NewDecoder(qResp.Body).Decode(&qRes)
	qResp.Body.Close()
	if qResp.StatusCode != 200 || qRes.Total != 1 || qRes.Items[0].Name != "boss.webm" {
		t.Fatalf("unexpected search result: %+v", qRes)
	}

	// 5. Library status & scan.
	stResp := env.req(http.MethodGet, "/api/library/status", tokA, nil)
	var st struct {
		Scanning bool `json:"scanning"`
		Counts   struct {
			Photo int `json:"photo"`
			Video int `json:"video"`
		} `json:"counts"`
	}
	_ = json.NewDecoder(stResp.Body).Decode(&st)
	stResp.Body.Close()
	if stResp.StatusCode != 200 || st.Counts.Photo != 2 || st.Counts.Video != 2 {
		t.Fatalf("unexpected library status: %+v", st)
	}
}
