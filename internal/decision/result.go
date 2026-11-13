package decision

type Result struct {
	Decision int
	Reason   Reason
	Signal   bool // kirim ke C# atau tidak
}
