package emmet

import "strings"

// Lorem ipsum text like Emmet's "lorem" generator, but with a seeded random
// source so the same abbreviation always gives the same text.

var mkLoremCommon = []string{"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipisicing", "elit"}

var mkLoremWords = strings.Fields(`exercitationem perferendis perspiciatis laborum eveniet sunt
iure nam nobis eum cum officiis excepturi odio consectetur quasi aut quisquam vel eligendi itaque
non odit tempore quaerat dignissimos facilis neque nihil expedita vitae vero ipsum nisi animi
cumque pariatur velit modi natus iusto eaque sequi illo sed ex et voluptatibus tempora veritatis
ratione assumenda incidunt nostrum placeat aliquid fuga provident praesentium rem necessitatibus
suscipit adipisci quidem possimus voluptas debitis sint accusantium unde sapiente voluptate qui
aspernatur laudantium soluta amet quo aliquam saepe culpa libero ipsa dicta reiciendis nesciunt
doloribus autem impedit minima maiores repudiandae ipsam obcaecati ullam enim totam delectus
ducimus quis voluptates dolores molestiae harum dolorem quia voluptatem molestias magni
distinctio omnis illum dolorum voluptatum ea quas quam corporis quae blanditiis atque deserunt
laboriosam earum consequuntur hic cupiditate quibusdam accusamus ut rerum error minus eius ab ad
nemo fugit officia at in id quos reprehenderit numquam iste fugiat sit inventore beatae
repellendus magnam recusandae quod explicabo doloremque aperiam consequatur asperiores commodi
optio dolor labore temporibus repellat veniam architecto est esse mollitia nulla a similique eos
alias dolore tenetur deleniti porro facere maxime corrupti`)

// mkRand is a small deterministic random source (splitmix64).
type mkRand struct{ s uint64 }

func mkNewRand(seed string) *mkRand {
	h := uint64(14695981039346656037)
	for i := 0; i < len(seed); i++ {
		h ^= uint64(seed[i])
		h *= 1099511628211
	}
	return &mkRand{s: h}
}

func (r *mkRand) next() uint64 {
	r.s += 0x9e3779b97f4a7c15
	z := r.s
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// between gives a number from lo to hi, both included.
func (r *mkRand) between(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + int(r.next()%uint64(hi-lo+1))
}

func mkCapitalize(w string) string {
	if w == "" {
		return w
	}
	return strings.ToUpper(w[:1]) + w[1:]
}

func (r *mkRand) sentence(words []string, end string) string {
	if len(words) > 0 {
		words[0] = mkCapitalize(words[0])
	}
	if end == "" {
		end = string("?!..."[r.between(0, 4)])
	}
	return strings.Join(words, " ") + end
}

func (r *mkRand) commas(words []string) []string {
	n := len(words)
	if n < 2 {
		return words
	}
	total := 0
	switch {
	case n > 3 && n <= 6:
		total = r.between(0, 1)
	case n > 6 && n <= 12:
		total = r.between(0, 2)
	case n > 12:
		total = r.between(1, 4)
	}
	for i := 0; i < total; i++ {
		p := r.between(0, n-2)
		if !strings.HasSuffix(words[p], ",") {
			words[p] += ","
		}
	}
	return words
}

func (r *mkRand) sample(n int) []string {
	if n > len(mkLoremWords) {
		n = len(mkLoremWords)
	}
	idx := make([]int, len(mkLoremWords))
	for i := range idx {
		idx[i] = i
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		j := r.between(i, len(idx)-1)
		idx[i], idx[j] = idx[j], idx[i]
		out[i] = mkLoremWords[idx[i]]
	}
	return out
}

// paragraph makes count words of lorem ipsum; common starts with "Lorem ipsum dolor sit amet".
func (r *mkRand) paragraph(count int, common bool) string {
	var parts []string
	total := 0
	if common {
		n := count
		if n > len(mkLoremCommon) {
			n = len(mkLoremCommon)
		}
		words := append([]string(nil), mkLoremCommon[:n]...)
		total = n
		parts = append(parts, r.sentence(words, "."))
	}
	for total < count {
		n := r.between(2, 30)
		if n > count-total {
			n = count - total
		}
		words := r.commas(r.sample(n))
		total += n
		parts = append(parts, r.sentence(words, ""))
	}
	return strings.Join(parts, " ")
}
