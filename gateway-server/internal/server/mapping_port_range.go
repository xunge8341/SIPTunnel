package server

import "siptunnel/internal/config"

func normalizeMappingPortRange(start, end, fallbackStart, fallbackEnd, sipPort, rtpStart, rtpEnd int) (int, int) {
	if start <= 0 {
		start = fallbackStart
	}
	if end <= 0 {
		end = fallbackEnd
	}
	if start > 0 && end > 0 && start <= end && end <= 65535 && !portsOverlap(start, end, sipPort, sipPort) && !portsOverlap(start, end, rtpStart, rtpEnd) {
		return start, end
	}
	span := end - start
	if span < 1 {
		span = config.DefaultMappingPortEnd - config.DefaultMappingPortStart
	}
	candidateStart := maxInt(config.DefaultMappingPortStart, rtpEnd+1)
	candidateEnd := candidateStart + span
	if sipPort >= candidateStart && sipPort <= candidateEnd {
		candidateStart = sipPort + 1
		candidateEnd = candidateStart + span
	}
	if candidateEnd > 65535 {
		candidateEnd = minInt(65535, rtpStart-1)
		candidateStart = candidateEnd - span
		if candidateStart < 1 {
			candidateStart = 1
		}
	}
	if candidateStart <= 0 || candidateEnd <= 0 || candidateStart > candidateEnd {
		return start, end
	}
	return candidateStart, candidateEnd
}

func portsOverlap(aStart, aEnd, bStart, bEnd int) bool {
	if aStart <= 0 || aEnd <= 0 || bStart <= 0 || bEnd <= 0 || aStart > aEnd || bStart > bEnd {
		return false
	}
	return aStart <= bEnd && bStart <= aEnd
}
