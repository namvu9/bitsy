package size

import "fmt"

// Size is the size of the file if bytes
type Size uint64

// Sizes
const (
	KiB = 1024
	MiB = 1024 * 1024
	GiB = 1024 * 1024 * 1024
)

// KiB returns the size of the file in Kibibytes (fs / 1024)
func (fs Size) KiB() float64 {
	return float64(fs) / KiB
}

// MiB returns the size of the file in Mebibytes (fs /
// 1024^2)
func (fs Size) MiB() float64 {
	return float64(fs) / MiB
}

// GiB returns the size of the file in Gibibytes (fs /
// 1024^3)
func (fs Size) GiB() float64 {
	return float64(fs) / GiB
}

func (fs Size) String() string {
	if fs < 1024 {
		return fmt.Sprintf("%d B", fs)
	}

	if fs < 1024*1024 {
		return fmt.Sprintf("%.2f KiB", fs.KiB())
	}

	if fs < 1024*1024*1024 {
		return fmt.Sprintf("%.2f MiB", fs.MiB())
	}

	return fmt.Sprintf("%.2f GiB", fs.GiB())
}
