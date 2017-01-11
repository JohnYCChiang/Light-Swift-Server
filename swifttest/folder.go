package swifttest

// The Folder type represents a container stored in an account
type Folder struct {
	Count int    `json:"count"`
	Bytes int    `json:"bytes"`
	Name  string `json:"name"`
}

// The Subdir type
type Subdir struct {
	Subdir string `json:"subdir"`
}
