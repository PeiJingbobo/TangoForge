# -*- coding: utf-8 -*-
import io

TEST = '''

func TestImport_DraftReviewAndEdit(t *testing.T) {
	srv, _ := apiServerWithLLM(t, importDoc)
	dir := importProjectViaAPI(t, srv)

	// 1. 导入 → 草稿。
	body, _ := json.Marshal(map[string]any{"content": "# 文档\\n", "source_file": "docs/review.md"})
	rec := uiReq(t, srv, http.MethodPost, "/api/import", dir, string(body))
	out := mustCode(t, rec, http.StatusOK, "import")
	var draftResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(out), &draftResp)
	draftID := draftResp.Data.ID

	// 2. GET 明细：含完整任务树（状态机 key / 优先级 / 标题）。
	rec = uiReq(t, srv, http.MethodGet, "/api/import/drafts/"+draftID, dir, "")
	out = mustCode(t, rec, http.StatusOK, "draft detail")
	for _, want := range []string{`"source_file":"docs/review.md"`, `"title":"导入任务A"`, `"status":"doing"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("明细缺 %s: %s", want, out)
		}
	}

	// 3. PUT 整体更新任务树（编辑保存）。
	edit := `{"tasks":[{"title":"导入任务A-改","description":"新描述","status":"todo","priority":4,"tags":["y"],"assignee":"PB","depends_on":[],"children":[]}]}`
	rec = uiReq(t, srv, http.MethodPut, "/api/import/drafts/"+draftID+"/tasks", dir, edit)
	mustCode(t, rec, http.StatusOK, "update tasks")
	rec = uiReq(t, srv, http.MethodGet, "/api/import/drafts/"+draftID, dir, "")
	out = mustCode(t, rec, http.StatusOK, "draft after edit")
	if !strings.Contains(out, "导入任务A-改") || !strings.Contains(out, `"priority":4`) {
		t.Fatalf("编辑未生效: %s", out)
	}

	// 4. 非法更新（title 空）→ 422。
	bad := `{"tasks":[{"title":"","status":"todo"}]}`
	rec = uiReq(t, srv, http.MethodPut, "/api/import/drafts/"+draftID+"/tasks", dir, bad)
	out = mustCode(t, rec, http.StatusUnprocessableEntity, "invalid update")
	if !strings.Contains(out, "title") {
		t.Fatalf("非法更新应 422: %s", out)
	}

	// 5. 确认导入使用编辑后的任务树。
	rec = uiReq(t, srv, http.MethodPost, "/api/import/drafts/"+draftID+"/confirm", dir, "")
	mustCode(t, rec, http.StatusOK, "confirm after edit")
	rec = uiReq(t, srv, http.MethodGet, "/api/tasks", dir, "")
	out = mustCode(t, rec, http.StatusOK, "tasks after confirm")
	if !strings.Contains(out, "导入任务A-改") {
		t.Fatalf("确认导入应使用编辑后标题: %s", out)
	}
}
'''

p = 'internal/api/handlers_imports_test.go'
s = io.open(p, encoding='utf-8').read()
if 'TestImport_DraftReviewAndEdit' not in s:
    s = s + TEST
    io.open(p, 'w', encoding='utf-8', newline='\n').write(s)
    print('test appended')
else:
    print('already present')
