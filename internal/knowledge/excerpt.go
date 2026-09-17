package knowledge

import (
	"sort"
	"strings"
)

// ExcerptAround returns a query-relevant slice of content (≈2*radius bytes,
// snapped to line boundaries), for oversized compiled guides — e.g. the
// 121/86-病种 罕见病 compilations — where the first 2000 runes are unrelated
// to the disease the retriever matched. Falls back to the head of content
// when no query window occurs in it.
func ExcerptAround(content, query string, radius int) string {
	if radius <= 0 {
		radius = 4800 // ≈1600 CJK runes
	}
	rs := []rune(content)
	if len(rs) <= 2*radius {
		return content
	}
	windows := cjkWindows(query, 2, 6)
	for _, s := range nhcSynonymList(query) {
		windows = append(windows, s)
	}
	var latin []string
	for _, t := range tokenize(query) {
		if !hasCJK(t) && len([]rune(t)) >= 2 {
			latin = append(latin, t)
		}
	}
	type hit struct{ pos, weight int }
	var hits []hit
	for _, w := range windows {
		if len([]rune(w)) < 2 {
			continue
		}
		off := 0
		for {
			i := strings.Index(content[off:], w)
			if i < 0 {
				break
			}
			hits = append(hits, hit{off + i, len(w)})
			off += i + len(w)
		}
	}
	lower := strings.ToLower(content)
	for _, t := range latin {
		lt := strings.ToLower(t)
		off := 0
		for {
			i := strings.Index(lower[off:], lt)
			if i < 0 {
				break
			}
			hits = append(hits, hit{off + i, len(t)})
			off += i + len(t)
		}
	}
	if len(hits) == 0 {
		return headExcerpt(rs, radius)
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].pos < hits[j].pos })

	// Densest cluster of hits within a 2*radius span.
	bestSum, bestLo, curSum, curLo := 0, 0, 0, 0
	hi := 0
	for lo := 0; lo < len(hits); lo++ {
		if hi < lo {
			hi, curSum = lo, hits[lo].weight
		}
		for hi+1 < len(hits) && hits[hi+1].pos-hits[lo].pos <= 2*radius {
			hi++
			curSum += hits[hi].weight
		}
		if curSum > bestSum {
			bestSum, bestLo, curLo = curSum, lo, hi
		}
		curSum -= hits[lo].weight
	}
	center := (hits[bestLo].pos + hits[curLo].pos) / 2
	start := center - radius
	end := center + radius
	if start < 0 {
		start = 0
	}
	if end > len(content) {
		end = len(content)
	}
	// Snap to line boundaries so excerpts start/end clean.
	if s := strings.LastIndexByte(content[:start], '\n'); s >= 0 {
		start = s + 1
	}
	if e := strings.IndexByte(content[end:], '\n'); e >= 0 {
		end += e
	} else {
		end = len(content)
	}
	return content[start:end]
}

// headExcerpt is the fallback excerpt: the head of content, snapped to a line.
func headExcerpt(rs []rune, radius int) string {
	limit := 2 * radius
	if len(rs) <= limit {
		return string(rs)
	}
	head := string(rs[:limit])
	if i := strings.LastIndexByte(head, '\n'); i > 0 {
		head = head[:i]
	}
	return head + "…"
}
