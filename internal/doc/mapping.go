package doc

// StepMap maps positions across one step, as prosemirror-transform's
// StepMap: ranges are (start, oldSize, newSize) triples.
type StepMap struct {
	ranges   []int
	inverted bool
}

var emptyStepMap = &StepMap{}

// NewStepMap returns a map of the given (start, oldSize, newSize) ranges.
func NewStepMap(ranges []int, inverted bool) *StepMap {
	if len(ranges) == 0 && !inverted {
		return emptyStepMap
	}
	return &StepMap{ranges: ranges, inverted: inverted}
}

// Ranges returns the map's ranges.
func (m *StepMap) Ranges() []int { return m.ranges }

// Inverted reports whether the map is inverted.
func (m *StepMap) Inverted() bool { return m.inverted }

const (
	delBefore = 1
	delAfter  = 2
	delAcross = 4
	delSide   = 8
)

// MapResult is the result of mapping a position.
type MapResult struct {
	Pos     int
	delInfo int
	recover int // -1: none
}

// Deleted reports whether the content on the position's side was deleted.
func (r MapResult) Deleted() bool { return r.delInfo&delSide > 0 }

// DeletedBefore reports whether the token before the position was deleted.
func (r MapResult) DeletedBefore() bool { return r.delInfo&(delBefore|delAcross) > 0 }

// DeletedAfter reports whether the token after the position was deleted.
func (r MapResult) DeletedAfter() bool { return r.delInfo&(delAfter|delAcross) > 0 }

// DeletedAcross reports whether a deleted range covered the position.
func (r MapResult) DeletedAcross() bool { return r.delInfo&delAcross > 0 }

// Mappable maps positions: a StepMap or a Mapping.
type Mappable interface {
	Map(pos, assoc int) int
	MapResult(pos, assoc int) MapResult
}

func makeRecover(index, offset int) int { return index + offset*65536 }
func recoverIndex(v int) int            { return v & 0xffff }
func recoverOffset(v int) int           { return (v - v&0xffff) / 65536 }

func (m *StepMap) recover(v int) int {
	diff, index := 0, recoverIndex(v)
	if !m.inverted {
		for i := 0; i < index; i++ {
			diff += m.ranges[i*3+2] - m.ranges[i*3+1]
		}
	}
	return m.ranges[index*3] + diff + recoverOffset(v)
}

// MapResult maps pos; assoc says which side it sticks to.
func (m *StepMap) MapResult(pos, assoc int) MapResult { return m.mapping(pos, assoc, false) }

// Map maps pos.
func (m *StepMap) Map(pos, assoc int) int { return m.mapping(pos, assoc, true).Pos }

func (m *StepMap) mapping(pos, assoc int, simple bool) MapResult {
	diff := 0
	oldIndex, newIndex := 1, 2
	if m.inverted {
		oldIndex, newIndex = 2, 1
	}
	for i := 0; i < len(m.ranges); i += 3 {
		start := m.ranges[i]
		if m.inverted {
			start -= diff
		}
		if start > pos {
			break
		}
		oldSize, newSize := m.ranges[i+oldIndex], m.ranges[i+newIndex]
		end := start + oldSize
		if pos <= end {
			side := assoc
			if oldSize != 0 {
				if pos == start {
					side = -1
				} else if pos == end {
					side = 1
				}
			}
			result := start + diff
			if side >= 0 {
				result += newSize
			}
			if simple {
				return MapResult{Pos: result, recover: -1}
			}
			rec := -1
			edge := end
			if assoc < 0 {
				edge = start
			}
			if pos != edge {
				rec = makeRecover(i/3, pos-start)
			}
			del := 0
			if oldSize != 0 {
				switch pos {
				case start:
					del |= delAfter
				case end:
					del |= delBefore
				default:
					del |= delAcross
				}
				if (assoc < 0 && pos != start) || (assoc >= 0 && pos != end) {
					del |= delSide
				}
			}
			return MapResult{Pos: result, delInfo: del, recover: rec}
		}
		diff += newSize - oldSize
	}
	return MapResult{Pos: pos + diff, recover: -1}
}

// ForEach calls f with the old and new extent of each changed range.
func (m *StepMap) ForEach(f func(oldStart, oldEnd, newStart, newEnd int)) {
	oldIndex, newIndex := 1, 2
	if m.inverted {
		oldIndex, newIndex = 2, 1
	}
	diff := 0
	for i := 0; i < len(m.ranges); i += 3 {
		start := m.ranges[i]
		oldStart, newStart := start, start+diff
		if m.inverted {
			oldStart, newStart = start-diff, start
		}
		oldSize, newSize := m.ranges[i+oldIndex], m.ranges[i+newIndex]
		f(oldStart, oldStart+oldSize, newStart, newStart+newSize)
		diff += newSize - oldSize
	}
}

// Invert returns the map that undoes this one.
func (m *StepMap) Invert() *StepMap { return &StepMap{ranges: m.ranges, inverted: !m.inverted} }

// Mapping is a pipeline of step maps with mirrors, as prosemirror-transform's
// Mapping: a mirror pairs a map with its inverse so positions deleted by one
// are recovered by the other.
type Mapping struct {
	maps     []*StepMap
	mirror   []int
	from, to int
	ownData  bool
}

// NewMapping returns a mapping of the maps.
func NewMapping(maps []*StepMap) *Mapping {
	return &Mapping{maps: maps, to: len(maps), ownData: maps == nil}
}

// Maps returns the mapping's step maps.
func (m *Mapping) Maps() []*StepMap { return m.maps }

// Slice returns a mapping of the maps from..to, sharing this one's data.
func (m *Mapping) Slice(from, to int) *Mapping {
	return &Mapping{maps: m.maps, mirror: m.mirror, from: from, to: to}
}

// AppendMap adds a map; mirrors, when not negative, names its mirror.
func (m *Mapping) AppendMap(sm *StepMap, mirrors int) {
	if !m.ownData {
		m.maps = append([]*StepMap(nil), m.maps...)
		if m.mirror != nil {
			m.mirror = append([]int(nil), m.mirror...)
		}
		m.ownData = true
	}
	m.maps = append(m.maps, sm)
	m.to = len(m.maps)
	if mirrors >= 0 {
		m.SetMirror(len(m.maps)-1, mirrors)
	}
}

// AppendMapping adds the maps of another mapping with its mirrors.
func (m *Mapping) AppendMapping(o *Mapping) {
	startSize := len(m.maps)
	for i := range o.maps {
		mirr := o.getMirror(i)
		if mirr >= 0 && mirr < i {
			m.AppendMap(o.maps[i], startSize+mirr)
		} else {
			m.AppendMap(o.maps[i], -1)
		}
	}
}

func (m *Mapping) getMirror(n int) int {
	for i := 0; i < len(m.mirror); i++ {
		if m.mirror[i] == n {
			if i%2 == 1 {
				return m.mirror[i-1]
			}
			return m.mirror[i+1]
		}
	}
	return -1
}

// SetMirror pairs maps n and k.
func (m *Mapping) SetMirror(n, k int) {
	if m.mirror == nil {
		m.mirror = []int{}
	}
	m.mirror = append(m.mirror, n, k)
}

func (m *Mapping) appendMappingInverted(o *Mapping) {
	total := len(m.maps) + len(o.maps)
	for i := len(o.maps) - 1; i >= 0; i-- {
		mirr := o.getMirror(i)
		if mirr >= 0 && mirr > i {
			m.AppendMap(o.maps[i].Invert(), total-mirr-1)
		} else {
			m.AppendMap(o.maps[i].Invert(), -1)
		}
	}
}

// Invert returns the mapping that undoes this one.
func (m *Mapping) Invert() *Mapping {
	inv := &Mapping{ownData: true}
	inv.appendMappingInverted(m)
	return inv
}

// Map maps pos through the mapping.
func (m *Mapping) Map(pos, assoc int) int {
	if m.mirror != nil {
		return m.mapping(pos, assoc, true).Pos
	}
	for i := m.from; i < m.to; i++ {
		pos = m.maps[i].Map(pos, assoc)
	}
	return pos
}

// MapResult maps pos and says what was deleted around it.
func (m *Mapping) MapResult(pos, assoc int) MapResult { return m.mapping(pos, assoc, false) }

func (m *Mapping) mapping(pos, assoc int, simple bool) MapResult {
	delInfo := 0
	for i := m.from; i < m.to; i++ {
		r := m.maps[i].MapResult(pos, assoc)
		if r.recover >= 0 {
			if corr := m.getMirror(i); corr >= 0 && corr > i && corr < m.to {
				i = corr
				pos = m.maps[corr].recover(r.recover)
				continue
			}
		}
		delInfo |= r.delInfo
		pos = r.Pos
	}
	return MapResult{Pos: pos, delInfo: delInfo, recover: -1}
}
