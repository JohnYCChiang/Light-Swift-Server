package swifttest

// orderedObjects holds a slice of objects that can be sorted
// by name.
type orderedObjects []*Object

func (s orderedObjects) Len() int {
	return len(s)
}
func (s orderedObjects) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s orderedObjects) Less(i, j int) bool {
	return s[i].Name < s[j].Name
}
