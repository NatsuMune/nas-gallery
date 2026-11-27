package main

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// --- JSON shapes (camelCase, per docs/API_CONTRACT.md) ------------------------

type itemJSON struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	ModTime    string `json:"modTime"`
	Width      *int   `json:"width"`
	Height     *int   `json:"height"`
	DurationMs *int64 `json:"durationMs"`
	Favorite   bool   `json:"favorite"`
	FolderID   string `json:"folderId"`
	ThumbURL   string `json:"thumbUrl"`
	StreamURL  string `json:"streamUrl"`
}

type folderJSON struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	ParentID     *string `json:"parentId"`
	RelativePath string  `json:"relativePath"`
	FileCount    int     `json:"fileCount"`
	ChildCount   int     `json:"childCount"`
	CoverItemID  *string `json:"coverItemId"`
}

// listResponse is the folder-items shape: "folder" is present (null for the
// virtual root).
type listResponse struct {
	Items    []itemJSON  `json:"items"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
	Total    int         `json:"total"`
	Folder   *folderJSON `json:"folder"`
}

// listResponseNoFolder is the favorites/search shape (no folder key).
type listResponseNoFolder struct {
	Items    []itemJSON `json:"items"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
	Total    int        `json:"total"`
}

const rootFolderID = "root"

func itemPublicID(id int64) string         { return "m_" + strconv.FormatInt(id, 10) }
func folderPublicID(id int64) string       { return "f_" + strconv.FormatInt(id, 10) }
func parseItemID(s string) (int64, bool)   { return parsePrefixedID("m_", s) }
func parseFolderID(s string) (int64, bool) { return parsePrefixedID("f_", s) }

func parsePrefixedID(prefix, s string) (int64, bool) {
	if !strings.HasPrefix(s, prefix) {
		return 0, false
	}
	id, err := strconv.ParseInt(s[len(prefix):], 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (app *App) itemJSONFromRow(it itemRow, favorite bool) itemJSON {
	j := itemJSON{
		ID:        itemPublicID(it.ID),
		Name:      it.Name,
		Type:      it.Type,
		Size:      it.Size,
		ModTime:   it.Mtime.UTC().Format(time.RFC3339),
		Favorite:  favorite,
		FolderID:  folderPublicID(it.FolderID),
		ThumbURL:  app.signMediaURL(itemPublicID(it.ID), "thumb", url.Values{"s": []string{"grid"}}, app.cfg.signedTTL()),
		StreamURL: app.signMediaURL(itemPublicID(it.ID), "stream", url.Values{}, app.cfg.signedTTL()),
	}
	if it.Width.Valid {
		j.Width = new(int)
		*j.Width = int(it.Width.Int64)
	}
	if it.Height.Valid {
		j.Height = new(int)
		*j.Height = int(it.Height.Int64)
	}
	if it.DurationMs.Valid {
		j.DurationMs = new(int64)
		*j.DurationMs = it.DurationMs.Int64
	}
	return j
}

// --- list query parsing -------------------------------------------------------

type listQuery struct {
	Sort     string
	Order    string
	Type     string
	Page     int
	PageSize int
}

// parseListQuery reads sort/order/type/page/pageSize with strict validation
// (invalid values → 400) and lenient pageSize clamping at the documented max.
func parseListQuery(w http.ResponseWriter, r *http.Request) (listQuery, bool) {
	q := r.URL.Query()
	lq := listQuery{Sort: "date", Order: "desc", Type: "all", Page: 1, PageSize: 100}

	if v := q.Get("sort"); v != "" {
		switch v {
		case "name", "date", "size", "type":
			lq.Sort = v
		default:
			writeErr(w, http.StatusBadRequest, "sort must be one of name, date, size, type")
			return lq, false
		}
	}
	if v := q.Get("order"); v != "" {
		switch v {
		case "asc", "desc":
			lq.Order = v
		default:
			writeErr(w, http.StatusBadRequest, "order must be asc or desc")
			return lq, false
		}
	}
	if v := q.Get("type"); v != "" {
		switch v {
		case "all", "photo", "video":
			lq.Type = v
		default:
			writeErr(w, http.StatusBadRequest, "type must be all, photo or video")
			return lq, false
		}
	}
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeErr(w, http.StatusBadRequest, "page must be a positive integer")
			return lq, false
		}
		lq.Page = n
	}
	if v := q.Get("pageSize"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeErr(w, http.StatusBadRequest, "pageSize must be a positive integer")
			return lq, false
		}
		if n > 500 {
			n = 500 // documented max
		}
		lq.PageSize = n
	}
	return lq, true
}

func (lq listQuery) storeOpts(userID int64) listOpts {
	typeFilter := lq.Type
	if typeFilter == "all" {
		typeFilter = ""
	}
	return listOpts{
		userID:     userID,
		typeFilter: typeFilter,
		sort:       lq.Sort,
		order:      lq.Order,
		page:       lq.Page,
		pageSize:   lq.PageSize,
	}
}

// --- folders -------------------------------------------------------------------

type folderResponse struct {
	Folder      *folderJSON  `json:"folder"`
	Children    []folderJSON `json:"children"`
	Breadcrumbs []folderJSON `json:"breadcrumbs"`
}

func (app *App) handleFolder(w http.ResponseWriter, r *http.Request, p params) {
	if p["id"] == rootFolderID {
		roots, err := app.store.rootFolders()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to list folders")
			return
		}
		children, err := app.childrenFolderJSONs(r.Context(), roots, "")
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to list folders")
			return
		}
		writeJSON(w, http.StatusOK, folderResponse{Folder: nil, Children: children, Breadcrumbs: []folderJSON{}})
		return
	}
	id, ok := parseFolderID(p["id"])
	if !ok {
		writeErr(w, http.StatusNotFound, "folder not found")
		return
	}
	f, err := app.store.folderByID(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to load folder")
		return
	}
	if f == nil {
		writeErr(w, http.StatusNotFound, "folder not found")
		return
	}

	// Breadcrumbs: virtual root + ancestors + self.
	crumbs, relPath, err := app.breadcrumbs(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to build breadcrumbs")
		return
	}

	childrenRows, err := app.store.childFolders(f.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list folders")
		return
	}
	children, err := app.childrenFolderJSONs(r.Context(), childrenRows, relPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list folders")
		return
	}
	self := crumbs[len(crumbs)-1]
	writeJSON(w, http.StatusOK, folderResponse{Folder: &self, Children: children, Breadcrumbs: crumbs})
}

// breadcrumbs walks the parent chain and returns [root, …ancestors, self],
// plus the folder's relative path from the virtual root (including the media
// root's own name).
func (app *App) breadcrumbs(ctx context.Context, f *folderRow) ([]folderJSON, string, error) {
	var chain []*folderRow // f, parent(f), …, mediaRoot
	cur := f
	for cur != nil {
		chain = append(chain, cur)
		if !cur.ParentID.Valid {
			break
		}
		parent, err := app.store.folderByID(cur.ParentID.Int64)
		if err != nil {
			return nil, "", err
		}
		if parent == nil {
			break
		}
		cur = parent
	}
	// Names from media root down to self.
	names := make([]string, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		names = append(names, chain[i].Name)
	}
	relPath := strings.Join(names, "/")

	// Root crumb: the virtual root has no real row.
	rootChildCount := 0
	if roots, err := app.store.rootFolders(); err == nil {
		rootChildCount = len(roots)
	}
	root := folderJSON{
		ID:           rootFolderID,
		Name:         "Home",
		ParentID:     nil,
		RelativePath: "",
		FileCount:    0,
		ChildCount:   rootChildCount,
		CoverItemID:  nil,
	}

	crumbs := []folderJSON{root}
	for i := len(chain) - 1; i >= 0; i-- {
		fr := chain[i]
		parentID := rootFolderID
		if fr.ParentID.Valid {
			parentID = folderPublicID(fr.ParentID.Int64)
		}
		fj := folderJSON{
			ID:           folderPublicID(fr.ID),
			Name:         fr.Name,
			ParentID:     &parentID,
			RelativePath: strings.Join(names[:len(chain)-i], "/"),
			FileCount:    0,
			ChildCount:   0,
			CoverItemID:  nil,
		}
		app.fillFolderStats(ctx, &fj, fr.ID)
		crumbs = append(crumbs, fj)
	}
	return crumbs, relPath, nil
}

// folderJSONs serializes a set of folders with counts and covers (relative
// paths are left empty for callers that lack parent context).
func (app *App) folderJSONs(ctx context.Context, rows []folderRow) ([]folderJSON, error) {
	ids := make([]int64, len(rows))
	for i, fr := range rows {
		ids[i] = fr.ID
	}
	files, children, err := app.store.folderCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	covers, err := app.store.coverItemIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]folderJSON, 0, len(rows))
	for _, fr := range rows {
		parentID := rootFolderID
		if fr.ParentID.Valid {
			parentID = folderPublicID(fr.ParentID.Int64)
		}
		out = append(out, folderJSON{
			ID:           folderPublicID(fr.ID),
			Name:         fr.Name,
			ParentID:     &parentID,
			RelativePath: "",
			FileCount:    files[fr.ID],
			ChildCount:   children[fr.ID],
			CoverItemID:  coverRef(covers[fr.ID]),
		})
	}
	return out, nil
}

// childrenFolderJSONs serializes direct children of a folder whose relative
// path is known, so each child's relativePath is parent + "/" + name (or just
// the name when the parent is the virtual root).
func (app *App) childrenFolderJSONs(ctx context.Context, rows []folderRow, parentRel string) ([]folderJSON, error) {
	out, err := app.folderJSONs(ctx, rows)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if parentRel == "" {
			out[i].RelativePath = out[i].Name
		} else {
			out[i].RelativePath = parentRel + "/" + out[i].Name
		}
	}
	return out, nil
}

func coverRef(id int64) *string {
	if id == 0 {
		return nil
	}
	s := itemPublicID(id)
	return &s
}

func (app *App) fillFolderStats(ctx context.Context, fj *folderJSON, id int64) {
	files, children, err := app.store.folderCounts(ctx, []int64{id})
	if err == nil {
		fj.FileCount = files[id]
		fj.ChildCount = children[id]
	}
	covers, err := app.store.coverItemIDs(ctx, []int64{id})
	if err == nil {
		fj.CoverItemID = coverRef(covers[id])
	}
}

// --- items --------------------------------------------------------------------

func (app *App) handleFolderItems(w http.ResponseWriter, r *http.Request, p params) {
	user := authUserFrom(r.Context())
	lq, ok := parseListQuery(w, r)
	if !ok {
		return
	}
	opts := lq.storeOpts(user.ID)

	var folder *folderJSON
	if p["id"] == rootFolderID {
		opts.rootScope = true // files directly in any media root
	} else {
		id, valid := parseFolderID(p["id"])
		if !valid {
			writeErr(w, http.StatusNotFound, "folder not found")
			return
		}
		f, err := app.store.folderByID(id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to load folder")
			return
		}
		if f == nil {
			writeErr(w, http.StatusNotFound, "folder not found")
			return
		}
		crumbs, _, err := app.breadcrumbs(r.Context(), f)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to load folder")
			return
		}
		self := crumbs[len(crumbs)-1]
		folder = &self
		opts.folderID = id
	}

	items, favs, total, err := app.store.listItems(r.Context(), opts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list items")
		return
	}
	app.respondItems(w, items, favs, total, opts, folder)
}

func (app *App) handleItem(w http.ResponseWriter, r *http.Request, p params) {
	user := authUserFrom(r.Context())
	id, ok := parseItemID(p["id"])
	if !ok {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	it, err := app.store.itemByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to load item")
		return
	}
	if it == nil {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	fav, err := app.store.IsFavorite(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to load item")
		return
	}
	writeJSON(w, http.StatusOK, app.itemJSONFromRow(*it, fav))
}

func (app *App) handleFavorite(w http.ResponseWriter, r *http.Request, p params) {
	user := authUserFrom(r.Context())
	id, ok := parseItemID(p["id"])
	if !ok {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	var body struct {
		Favorite *bool `json:"favorite"`
	}
	if !readJSONBody(w, r, &body) {
		return
	}
	if body.Favorite == nil {
		writeErr(w, http.StatusBadRequest, "favorite (boolean) is required")
		return
	}
	exists, err := app.store.SetFavorite(r.Context(), user.ID, id, *body.Favorite)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update favorite")
		return
	}
	if !exists {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"favorite": *body.Favorite})
}

func (app *App) handleFavorites(w http.ResponseWriter, r *http.Request, _ params) {
	user := authUserFrom(r.Context())
	lq, ok := parseListQuery(w, r)
	if !ok {
		return
	}
	opts := lq.storeOpts(user.ID)
	opts.favoriteOf = user.ID
	app.respondItemList(w, r, opts)
}

func (app *App) handleSearch(w http.ResponseWriter, r *http.Request, _ params) {
	user := authUserFrom(r.Context())
	q := r.URL.Query().Get("q")
	if q == "" {
		writeErr(w, http.StatusBadRequest, "q is required (min 1 character)")
		return
	}
	lq, ok := parseListQuery(w, r)
	if !ok {
		return
	}
	opts := lq.storeOpts(user.ID)
	opts.search = q
	app.respondItemList(w, r, opts)
}

// respondItemList runs the query and answers with the folder-less list shape
// (favorites, search).
func (app *App) respondItemList(w http.ResponseWriter, r *http.Request, opts listOpts) {
	items, favs, total, err := app.store.listItems(r.Context(), opts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list items")
		return
	}
	out := make([]itemJSON, 0, len(items))
	for i, it := range items {
		out = append(out, app.itemJSONFromRow(it, favs[i]))
	}
	writeJSON(w, http.StatusOK, listResponseNoFolder{
		Items: out, Page: opts.page, PageSize: opts.pageSize, Total: total,
	})
}

func (app *App) respondItems(w http.ResponseWriter, items []itemRow, favs []bool, total int, opts listOpts, folder *folderJSON) {
	out := make([]itemJSON, 0, len(items))
	for i, it := range items {
		fav := false
		if i < len(favs) {
			fav = favs[i]
		}
		out = append(out, app.itemJSONFromRow(it, fav))
	}
	writeJSON(w, http.StatusOK, listResponse{
		Items: out, Page: opts.page, PageSize: opts.pageSize, Total: total, Folder: folder,
	})
}

// --- library -------------------------------------------------------------------

func (app *App) handleScan(w http.ResponseWriter, r *http.Request, _ params) {
	app.scanner.Trigger()
	writeJSON(w, http.StatusOK, map[string]bool{"scanning": true})
}

func (app *App) handleStatus(w http.ResponseWriter, r *http.Request, _ params) {
	photos, videos, err := app.store.typeCounts(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to load status")
		return
	}
	last, has, err := app.store.LastScanAt()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to load status")
		return
	}
	var lastStr *string
	if has {
		s := last.UTC().Format(time.RFC3339)
		lastStr = &s
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"scanning":   app.scanner.IsScanning(),
		"lastScanAt": lastStr,
		"counts":     map[string]int{"photo": photos, "video": videos},
	})
}
