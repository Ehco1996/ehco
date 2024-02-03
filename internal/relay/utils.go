package relay

func inArray(ele string, array []string) bool {
	for _, v := range array {
		if v == ele {
			return true
		}
	}
	return false
}
