package slice

// Reverse reverses any slice in place
// Taken from https://github.com/faiface/generics/blob/8cf65f0b43803410724d8c671cb4d328543ba07d/examples/sliceutils/sliceutils.go
func Reverse[E any](s []E) []E {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}
