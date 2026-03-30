package entities

type MediaSegment struct {
	url      string
	sequence int
	duration float64
}

func NewMediaSegment(url string, sequence int, duration float64) MediaSegment {
	return MediaSegment{
		url:      url,
		sequence: sequence,
		duration: duration,
	}
}

func (m MediaSegment) URL() string       { return m.url }
func (m MediaSegment) Sequence() int     { return m.sequence }
func (m MediaSegment) Duration() float64 { return m.duration }

type HLSPlaylist struct {
	resolution     string
	targetDuration float64
	segments       []MediaSegment
}

func NewHLSPlaylist(resolution string, targetDuration float64, segments []MediaSegment) HLSPlaylist {
	segmentsCopy := make([]MediaSegment, len(segments))
	copy(segmentsCopy, segments)

	return HLSPlaylist{
		resolution:     resolution,
		targetDuration: targetDuration,
		segments:       segmentsCopy,
	}
}

func (h HLSPlaylist) Resolution() string      { return h.resolution }
func (h HLSPlaylist) TargetDuration() float64 { return h.targetDuration }
func (h HLSPlaylist) Segments() []MediaSegment {
	segmentsCopy := make([]MediaSegment, len(h.segments))
	copy(segmentsCopy, h.segments)
	return segmentsCopy
}
