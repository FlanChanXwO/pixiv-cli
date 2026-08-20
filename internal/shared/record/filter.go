package record

import "encoding/json"

// RecordFilter 描述基于稳定记录字段的本地筛选条件。
// MinViews 和 MinPageCount 为 nil 时不筛选对应字段。
type RecordFilter struct {
	ID           string
	Type         string
	Tags         []string
	MinViews     *int64
	MinPageCount *int64
}

// Matches 判断记录是否满足所有已设置的筛选条件，不修改记录内容。
func (r Record) Matches(filter RecordFilter) bool {
	if filter.ID != "" && r.ID() != filter.ID {
		return false
	}
	if filter.Type != "" && r.Type() != filter.Type {
		return false
	}
	if len(filter.Tags) > 0 && !recordHasTags(r.fields["tags"], filter.Tags) {
		return false
	}
	if filter.MinViews != nil {
		views, ok := recordInt(r.fields["total_view"])
		if !ok {
			views, ok = recordInt(r.fields["total_views"])
		}
		if !ok {
			views, ok = recordInt(r.fields["views"])
		}
		if !ok || views < *filter.MinViews {
			return false
		}
	}
	if filter.MinPageCount != nil {
		pageCount, ok := recordInt(r.fields["page_count"])
		if !ok || pageCount < *filter.MinPageCount {
			return false
		}
	}
	return true
}

func recordHasTags(raw any, wanted []string) bool {
	rawTags, ok := raw.([]any)
	if !ok {
		return false
	}
	found := make(map[string]struct{}, len(rawTags))
	for _, rawTag := range rawTags {
		if tag, ok := rawTag.(string); ok {
			if tag != "" {
				found[tag] = struct{}{}
			}
			continue
		}
		objectTag, ok := rawTag.(map[string]any)
		if !ok {
			continue
		}
		if name := stringField(objectTag["name"]); name != "" {
			found[name] = struct{}{}
		}
	}
	for _, tag := range wanted {
		if _, ok := found[tag]; !ok {
			return false
		}
	}
	return true
}

func recordInt(raw any) (int64, bool) {
	number, ok := raw.(json.Number)
	if !ok {
		return 0, false
	}
	value, err := number.Int64()
	if err != nil {
		return 0, false
	}
	return value, true
}
