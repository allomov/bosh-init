package fakes

type FakeUI struct {
	Said   []string
	Errors []string
}

func (ui *FakeUI) ErrorLinef(message string) {
	ui.Errors = append(ui.Errors, message)
}

func (ui *FakeUI) PrintLinef(message string) {
	ui.Said = append(ui.Said, message)
}

func (ui *FakeUI) BeginLinef(message string) {
	ui.Said = append(ui.Said, message)
}

func (ui *FakeUI) EndLinef(message string) {
	ui.Said = append(ui.Said, message)
}
