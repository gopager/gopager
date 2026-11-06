package ingest

// validPriorities reports whether the priorities of the request are exactly the set 0..n-1, in
// any request order. Requiring a contiguous set keeps the stored order unambiguous and makes a
// forgotten or duplicated entry a visible error instead of a silent tie-break.
func validPriorities(input []GatewayNetworkInput) bool {
	seen := make([]bool, len(input))
	for _, item := range input {
		if item.Priority < 0 || item.Priority >= len(input) || seen[item.Priority] {
			return false
		}
		seen[item.Priority] = true
	}
	return true
}
