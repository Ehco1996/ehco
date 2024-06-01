package metric_reader

import "regexp"

// parse disk name from device path,such as:
// e.g. /dev/disk1s1 -> disk1
// e.g. /dev/disk1s2 -> disk1
// e.g. ntfs://disk1s1 -> disk1
// e.g. ntfs://disk1s2 -> disk1
// e.g. /dev/sda1 -> sda
// e.g. /dev/sda2 -> sda
var diskNameRegex = regexp.MustCompile(`/dev/disk(\d+)|ntfs://disk(\d+)|/dev/sd[a-zA-Z]`)

func getDiskName(devicePath string) string {
	matches := diskNameRegex.FindStringSubmatch(devicePath)
	for _, match := range matches {
		if match != "" {
			return match
		}
	}
	return ""
}
