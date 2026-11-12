package lane

import (
	"context"
	"regexp"
	"strings"
)

// ModelInfo is one catalog entry.
type ModelInfo struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Blurb              string `json:"blurb,omitempty"`     // 一行小介绍：社区评价 + 性能定位 + 推荐度
	Wire               string `json:"wire"`                // chat | responses | messages | systemone
	Channel            string `json:"channel,omitempty"`   // "" | "kilo" — which free source serves this id
	SystemOne          bool   `json:"systemOne,omitempty"` // decision model, not chat-capable
	Vision             bool   `json:"vision"`
	AudioInput         bool   `json:"audioInput,omitempty"`
	FileInput          bool   `json:"fileInput,omitempty"`
	Reasoning          bool   `json:"reasoning"`
	ContextWindow      int    `json:"contextWindow"`
	MaxOutput          int    `json:"maxOutput"`
	CanDisableThinking bool   `json:"canDisableThinking"`
	RegionSensitive    bool   `json:"regionSensitive"`
}

// Channel tags for the catalog's free sources. The empty channel is the
// OpenCode Zen lane itself; ChannelKilo is the keyless Kilo free pool;
// ChannelWorkBuddy is the WorkBuddy 账号渠道 (intl); ChannelCodeArts is the
// Huawei Cloud CodeArts 账号渠道, the credential-bearing fourth source.
const (
	// ChannelZen names the OpenCode Zen lane in the per-channel files and in
	// the dashboard. The catalog leaves a zen row's Channel empty (the empty
	// channel *is* zen), so this constant exists for the places that need the
	// id as a string: channels/<id>.json, the admin state, the wall.
	ChannelZen       = "zen"
	ChannelKilo      = "kilo"
	ChannelWorkBuddy = "workbuddy"
	ChannelCodeArts  = "codearts"
	ChannelZCode     = "zcode"
	ChannelTrae      = "trae"
	// ChannelQoder names the sidecar-hosted Qoder pool. It has no catalog rows
	// of its own: its models arrive as a custom provider entry the gateway
	// syncs, which is why the wall reads its state straight from the sidecar.
	ChannelQoder = "qoder"
)

var alwaysFree = map[string]bool{"union-alpha": true, "space-bunny-free": true}

var freeLaneRe = regexp.MustCompile(`(?:^|[-_])free(?:$|[-_.])`)

// IsFreeLane: is this id on the free lane? The gateway listing mixes paid and
// free ids; only these answer without a per-user key.
func IsFreeLane(modelID string) bool {
	base := BaseModelId(modelID)
	if alwaysFree[base] {
		return true
	}
	return freeLaneRe.MatchString(base)
}

type capability struct {
	match              *regexp.Regexp
	vision             bool
	audio              bool
	file               bool
	reasoning          bool
	contextWindow      int
	maxOutput          int
	canDisableThinking bool
}

// capabilities is the local baseline: contextWindow/maxOutput are the
// provider's published capacities (checked against official model pages and
// reviews, kept in sync with the blurbs below); vision is what the lane
// actually accepted under a direct probe, not what a model card claims.
// audio/file stay false until verified (a live probe or the AI tagger upgrades
// them) — an unverified modality must never receive that modality's traffic.
//
// The 2026-10-15 rows (deepseek-v4.1-flash-free, ll-alpha-free,
// nemotron-3.5-super-vl-free, glm-5.3-flash-free) are measured, not guessed:
// each was probed against the live lane with the v2 fingerprint — context by
// sweeping prompt sizes until the upstream refused, output by raising
// max_tokens until it refused, vision by sending a 1×1 PNG. Context figures
// are the largest accepted size rounded down to a sane boundary; where the
// sweep was inconclusive (glm's large requests kept drawing transient 429s)
// the row carries only what was actually proven.
var capabilities = []capability{
	{regexp.MustCompile(`^mimo.*v2\.6`), true, false, false, true, 1048576, 131072, false},
	{regexp.MustCompile(`^mimo.*v2\.5`), true, false, false, true, 1048576, 131072, false},
	{regexp.MustCompile(`^mimo`), true, false, false, true, 262144, 131072, true},
	{regexp.MustCompile(`^muse.?spark`), true, false, false, true, 1048576, 943718, true},
	{regexp.MustCompile(`^step`), false, false, false, true, 1000000, 64000, true},
	{regexp.MustCompile(`^nemotron.*super.?vl`), true, false, false, true, 1048576, 262144, true},
	{regexp.MustCompile(`^nemotron`), false, false, false, true, 128000, 32768, true},
	{regexp.MustCompile(`^ling`), false, false, false, true, 262144, 32768, true},
	{regexp.MustCompile(`^space.?bunny`), true, false, false, true, 1048576, 65536, true},
	{regexp.MustCompile(`^union`), true, false, false, false, 262144, 131072, true},
	{regexp.MustCompile(`^deepseek.*v4\.1`), true, false, false, true, 1048576, 262144, true},
	{regexp.MustCompile(`^deepseek`), false, false, false, true, 1048576, 65536, true},
	{regexp.MustCompile(`^ll.?alpha`), true, false, false, true, 1048576, 131072, true},
	{regexp.MustCompile(`^glm`), false, false, false, true, 524288, 262144, true},
	{regexp.MustCompile(`^longcat`), true, false, false, true, 1048576, 262144, true},
	{regexp.MustCompile(`^fledge`), false, false, false, true, 131072, 32768, true},
	{regexp.MustCompile(`^jev`), false, false, false, false, 32768, 4096, true},
}

// regionSensitiveRes marks models a CN egress cannot reach: the upstream
// answers RegionError rather than a quota refusal, and routing to them just
// wastes a turn. beam-free refused from a CN probe on 2026-10-15.
var regionSensitiveRes = []*regexp.Regexp{
	regexp.MustCompile(`^muse.?spark`),
	regexp.MustCompile(`^beam`),
}

var displayNames = map[string]string{
	"mimo-v2.6-flash-free":            "MiMo V2.6 Flash",
	"mimo-v2.5-free":                  "MiMo V2.5",
	"muse-spark-1.3-contributor-free": "Muse Spark 1.3",
	"muse-spark-1.2-contributor-free": "Muse Spark 1.2",
	"nemotron-3-ultra-free":           "Nemotron 3 Ultra",
	"nemotron-3.5-lightning-free":     "Nemotron 3.5 Lightning",
	"nemotron-3.5-super-vl-free":      "Nemotron 3.5 Super VL",
	"ling-3.0-flash-fin-free":         "Ling 3.0 Flash Fin",
	"space-bunny-free":                "Space Bunny",
	"union-alpha":                     "Union Alpha",
	"deepseek-v4-flash-free":          "DeepSeek V4 Flash",
	"deepseek-v4.1-flash-free":        "DeepSeek V4.1 Flash",
	"ll-alpha-free":                   "LL Alpha",
	"glm-5.3-flash-free":              "GLM 5.3 Flash",
	"beam-free":                       "Beam",
	"jev-1.13-free":                   "Jev 1.13",
}

// blurbs are curated one-liners (community reviews + benchmarks, GPT series as
// the reference point) shown on the model page. Sources: Artificial Analysis
// 智能指数, Code Arena, SemiAnalysis, r/opencode, opencode.ai 用量榜.
var blurbs = map[string]string{
	"mimo-v2.6-flash-free":            "Lá bài chủ lực mã nguồn mở của Xiaomi: đứng đầu chỉ số trí tuệ trọng số mở, top 10 Code Arena, tổng thể tiệm cận dòng chủ lực cấp GPT-5, ngữ cảnh 1M. Tổng hợp khuyến nghị ★★★★★ (chủ lực mặc định)",
	"mimo-v2.5-free":                  "Flagship thế hệ trước của Xiaomi, ngữ cảnh 1M, agent ổn định; năng lực ở khoảng GPT-4.5~5, đã bị V2.6 thay thế toàn diện. Khuyến nghị ★★★ (dự phòng)",
	"muse-spark-1.3-contributor-free": "Mô hình chuyên lập trình của Meta, chính thức đối sánh GPT-5.6 Sol, trần lập trình cao; nhưng đo thực tế egress Trung Quốc bị cổng khu vực chặn. Khuyến nghị ★★★★ (có egress nước ngoài hãy bật)",
	"muse-spark-1.2-contributor-free": "Mô hình lập trình thế hệ trước của Muse Spark, cùng định vị cấp GPT-5; egress Trung Quốc bị giới hạn khu vực. Khuyến nghị ★★★",
	"nemotron-3-ultra-free":           "Flagship mã nguồn mở của NVIDIA, danh tiếng trung bình (SemiAnalysis cho rằng kém hơn nhóm đầu mã nguồn mở Trung Quốc), khoảng cấp GPT-4.5; kết nối thẳng nội địa (Trung Quốc) ổn định nhất. Khuyến nghị ★★★ (lựa chọn ổn định)",
	"nemotron-3.5-lightning-free":     "Nemotron 3.5 bản nhẹ nhanh, hợp hỏi đáp nhanh và tác vụ ngắn, khoảng cấp GPT-4.5. Khuyến nghị ★★★",
	"ling-3.0-flash-fin-free":         "Ant Ling 3.0 Flash: ngữ cảnh 256K, trọng số mở, nhẹ nhanh đáng tin, năng lực khoảng GPT-4.5~5. Khuyến nghị ★★★★",
	"ling-3.1-flash-free":             "Ant Ling 3.1 bản lặp, cùng dòng nhẹ nhanh, tác vụ hằng ngày mượt mà. Khuyến nghị ★★★★",
	"space-bunny-free":                "Mô hình ẩn danh ngữ cảnh 1M, đứng đầu bảng sử dụng OpenCode (63T tokens/tuần), cộng đồng đánh giá tốt, lai lịch chưa công bố. Khuyến nghị ★★★★☆",
	"union-alpha":                     "Mô hình miễn phí được cộng đồng r/opencode hay gợi ý, tổng thể khoảng cấp GPT-4.5+. Khuyến nghị ★★★★",
	"deepseek-v4-flash-free":          "DeepSeek V4 Flash: ưu tiên tốc độ, ký tự đầu nhanh hơn 40-60%, xuất ~214 tok/s, ngữ cảnh 1M, chính thức tuyên bố trải nghiệm vượt toàn diện bản Pro của hãng, tổng thể ngang cấp GPT-5. Khuyến nghị ★★★★★ (vua tốc độ)",
	"deepseek-v4.1-flash-free":        "DeepSeek V4.1 Flash: flagship mới sẽ gỡ khỏi kệ sau 10-15 ngày, đo thực tế ngữ cảnh 1M+, trần xuất 256K, nhận đầu vào hình ảnh — một trong những mô hình hào phóng nhất về ngữ cảnh và đầu ra ở làn miễn phí hiện nay. Khuyến nghị ★★★★★ (ưu tiên số một cho tài liệu dài / đầu ra lớn)",
	"ll-alpha-free":                   "Mô hình miễn phí chưa rõ nguồn gốc, đo thực tế ngữ cảnh 1M, xuất 128K, nhận hình ảnh; năng lực chưa hiệu chuẩn, tạm thử ở khoảng GPT-4.5~5. Khuyến nghị ★★★ (dùng thử)",
	"nemotron-3.5-super-vl-free":      "Bản tăng cường đa phương thức của NVIDIA (VL = Vision-Language): đo thực tế ngữ cảnh 1M, xuất 256K, nhận hình ảnh, xem ảnh và tài liệu dài đều xử lý được. Khuyến nghị ★★★★ (đa phương thức đáng giá)",
	"glm-5.3-flash-free":              "GLM 5.3 Flash bản nhẹ của Zhipu, đo thực tế trần xuất 256K; trong lúc đo upstream thỉnh thoảng trả 429 (Endpoint unavailable), độ khả dụng dao động lớn. Khuyến nghị ★★★ (xem trạng thái rồi dùng)",
	"beam-free":                       "Mô hình mới lên kệ, nhưng egress Trung Quốc bị cổng khu vực của upstream chặn thẳng (RegionError), chỉ dùng được khi có egress nước ngoài. Khuyến nghị ★★ (có egress nước ngoài hãy bật)",
	"longcat-2.5-preview-free":        "LongCat 2.5 bản xem trước của Meituan: MoE thưa động cấp nghìn tỷ tham số, ngữ cảnh hàng triệu, huấn luyện toàn trình bằng sức tính nội địa. Khuyến nghị ★★★★",
	"fledge-alpha-free":               "Gương mặt bí ẩn mới lên kệ, cộng đồng còn ít đánh giá; tạm thử ở cấp GPT-4.5. Khuyến nghị ★★★ (dùng thử)",
	"jev-1.13-free":                   "Mô hình phán định System One của TypeSafe AI: chỉ trả ba loại quyết định có cấu trúc choice/score/noul, không sinh nội dung, dùng cho Agent làm lớp phán định ý định/cảm xúc. Khuyến nghị ★★ (đừng dùng để trò chuyện)",
}

// DisplayModelName turns a bare upstream id into something a picker can show.
func DisplayModelName(modelID string) string {
	base := BaseModelId(modelID)
	if n, ok := displayNames[base]; ok {
		return n
	}
	words := strings.FieldsFunc(base, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	for i, w := range words {
		if len(w) > 0 && !(w[0] >= '0' && w[0] <= '9') {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func capabilitiesFor(base string) capability {
	for _, c := range capabilities {
		if c.match.MatchString(base) {
			return c
		}
	}
	// Nothing is known about this model, so nothing is claimed: a guessed
	// reasoning:true showed up on the model page as a verified 思考 badge.
	return capability{vision: false, audio: false, file: false, reasoning: false, contextWindow: 131072, maxOutput: 32768, canDisableThinking: true}
}

// CapabilityMatched reports whether the local table states anything about a
// model — false means capabilitiesFor() answered with its defaults.
func CapabilityMatched(base string) bool {
	for _, c := range capabilities {
		if c.match.MatchString(base) {
			return true
		}
	}
	return false
}

// BuildCatalog merges the upstream listing with the local capability table,
// keeping listing order and deduplicating by base id.
func BuildCatalog(ids []string) []ModelInfo {
	seen := map[string]bool{}
	entries := []ModelInfo{}
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || !IsFreeLane(id) {
			continue
		}
		base := BaseModelId(id)
		if seen[base] {
			continue
		}
		seen[base] = true
		caps := capabilitiesFor(base)
		wire := "chat"
		if WireFor(base) != "chat" {
			wire = WireFor(base)
		}
		regionSensitive := false
		for _, re := range regionSensitiveRes {
			if re.MatchString(base) {
				regionSensitive = true
			}
		}
		entries = append(entries, ModelInfo{
			ID:                 base,
			Name:               DisplayModelName(base),
			Blurb:              blurbs[base],
			Wire:               wire,
			SystemOne:          wire == "systemone",
			Vision:             caps.vision,
			AudioInput:         caps.audio,
			FileInput:          caps.file,
			Reasoning:          caps.reasoning,
			ContextWindow:      caps.contextWindow,
			MaxOutput:          caps.maxOutput,
			CanDisableThinking: caps.canDisableThinking,
			RegionSensitive:    regionSensitive,
		})
	}
	return entries
}

// FallbackCatalogIDs covers a cold start with no network. Mirrors the live
// free roster as of 2026-10-15 (mimo-v2.5 / ling-3.0-flash-fin / nemotron-3-ultra
// / deepseek-v4-flash have since left the upstream listing).
var FallbackCatalogIDs = []string{
	"mimo-v2.6-flash-free", "deepseek-v4.1-flash-free", "space-bunny-free",
	"nemotron-3.5-lightning-free", "nemotron-3.5-super-vl-free",
	"ll-alpha-free", "glm-5.3-flash-free", "longcat-2.5-preview-free",
	"muse-spark-1.3-contributor-free", "muse-spark-1.2-contributor-free",
	"union-alpha", "beam-free", "jev-1.13-free",
}

// ListingMeta is one upstream model row: its id plus whatever the provider
// itself declares about it. Zero bools mean "not stated" unless the matching
// *Declared flag says otherwise.
type ListingMeta struct {
	ID             string `json:"id"`
	Name           string `json:"name,omitempty"` // the provider's own display name
	ContextWindow  int    `json:"contextWindow,omitempty"`
	MaxOutput      int    `json:"maxOutput,omitempty"`
	Vision         bool   `json:"vision,omitempty"`
	Audio          bool   `json:"audio,omitempty"`
	File           bool   `json:"file,omitempty"`
	Reasoning      bool   `json:"reasoning,omitempty"`
	InputDeclared  bool   `json:"inputDeclared,omitempty"`  // input_modalities were stated
	OutputDeclared bool   `json:"outputDeclared,omitempty"` // output_modalities were stated

	// Efforts are the thinking rungs the provider publishes for this model, in
	// the provider's own naming, lightest first. EffortDefault is whichever of
	// them the provider turns on when the caller states nothing. Both empty
	// means the listing said nothing about thinking levels — the gateway then
	// falls back to its own budget ladder instead of claiming rungs the
	// upstream has never heard of.
	Efforts       []string `json:"efforts,omitempty"`
	EffortDefault string   `json:"effortDefault,omitempty"`
}

// dig walks a decoded-JSON path: a string step is an object key, an int step
// indexes an array. Any missing or mis-typed step yields nil.
func dig(v any, path ...any) any {
	cur := v
	for _, step := range path {
		switch s := step.(type) {
		case string:
			obj, ok := cur.(map[string]any)
			if !ok {
				return nil
			}
			cur = obj[s]
		case int:
			list, ok := cur.([]any)
			if !ok || s >= len(list) {
				return nil
			}
			cur = list[s]
		}
		if cur == nil {
			return nil
		}
	}
	return cur
}

func numberAt(v any, path ...any) int {
	if n, ok := dig(v, path...).(float64); ok && n > 0 {
		return int(n)
	}
	return 0
}

func stringAt(v any, path ...any) string {
	s, _ := dig(v, path...).(string)
	return s
}

// firstString returns the first key that holds a non-empty string.
func firstString(row map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := row[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// applyModalitySchema reads the per-modality declaration shape some providers
// publish (schema_version 2.4 style): input_modalities carries the context
// window and the accepted media types, output_modalities carries the output cap
// and whether a reasoning switch exists. Absent the block, nothing is claimed —
// an unlisted modality is then "unknown", not "refused".
func applyModalitySchema(row map[string]any, meta *ListingMeta) {
	if inputs, ok := row["input_modalities"].([]any); ok {
		meta.InputDeclared = true
		for _, item := range inputs {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			switch strings.ToLower(stringAt(m, "type")) {
			case "image":
				meta.Vision = true
			case "audio":
				meta.Audio = true
			case "file", "document", "pdf":
				meta.File = true
			}
			if n := numberAt(m, "supported_inputs", "max_context_length", "value"); n > 0 && meta.ContextWindow == 0 {
				meta.ContextWindow = n
			}
		}
	}
	outputs, ok := row["output_modalities"].([]any)
	if !ok {
		return
	}
	meta.OutputDeclared = true
	for _, item := range outputs {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if dig(m, "supported_parameters", "reasoning") != nil {
			meta.Reasoning = true
		}
		n := numberAt(m, "max_length", "value")
		if n == 0 {
			n = numberAt(m, "supported_parameters", "max_tokens", "max")
		}
		if n > 0 && meta.MaxOutput == 0 {
			meta.MaxOutput = n
		}
	}
}

// Declared-capacity field names seen in the wild: NVIDIA NIM splits input and
// output caps, OpenRouter publishes one total window plus a completion cap.
// Order is a preference — an input cap is the tighter, more honest bound for
// routing than a viewer-wide token allowance.
var (
	declaredContextKeys = []string{"max_input_tokens", "context_length", "viewer_total_token_limit", "context_window"}
	declaredOutputKeys  = []string{"max_output_tokens", "max_completion_tokens"}
)

func declaredInt(row map[string]any, keys ...string) int {
	for _, k := range keys {
		// encoding/json numbers everything as float64.
		if n, ok := row[k].(float64); ok && n > 0 {
			return int(n)
		}
	}
	return 0
}

// listingRows returns the model rows of a listing payload, or nil when the
// payload carries no recognisable list.
func listingRows(payload map[string]any) []any {
	if d, ok := payload["data"].([]any); ok {
		return d
	}
	if d, ok := payload["models"].([]any); ok {
		return d
	}
	return nil
}

// ParseListingMeta reads the listing keeping each row's declared capacities.
// Rows without an id are dropped; rows without numbers are kept, so a caller
// can tell "the provider published nothing" apart from "the provider was
// unreachable".
func ParseListingMeta(payload map[string]any) []ListingMeta {
	rows := []ListingMeta{}
	for _, row := range listingRows(payload) {
		switch t := row.(type) {
		case string:
			if id := strings.TrimSpace(t); id != "" {
				rows = append(rows, ListingMeta{ID: id})
			}
		case map[string]any:
			id, _ := t["id"].(string)
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			rows = append(rows, ListingMeta{
				ID:            id,
				Name:          firstString(t, "name", "display_name"),
				ContextWindow: declaredInt(t, declaredContextKeys...),
				MaxOutput:     declaredInt(t, declaredOutputKeys...),
			})
			applyModalitySchema(t, &rows[len(rows)-1])
		}
	}
	return rows
}

// ParseListing accepts {"data":[{"id":…}]}, {"models":[…]} or a bare array.
func ParseListing(payload map[string]any) []string {
	ids := []string{}
	for _, row := range listingRows(payload) {
		switch t := row.(type) {
		case string:
			ids = append(ids, t)
		case map[string]any:
			if id, ok := t["id"].(string); ok && id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// FetchListing pulls the authoritative id set from the gateway.
func FetchListing(ctx context.Context) ([]string, error) {
	var payload map[string]any
	err := GetJSON(ctx, "/zen/v1/models", SessionForConversation("catalog:openki"), MintRequestId(0), &payload)
	if err != nil {
		return nil, err
	}
	return ParseListing(payload), nil
}
