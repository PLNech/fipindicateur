//go:build !darwin

package ui

// No native slider or scroll wheel off darwin: Windows draws the volume
// submenu with preset checkboxes (plus the zenity slider where the binary
// exists) and Linux builds its menu on the sni dbusmenu, which already wires
// its own Scroll callback. The seam the shared code calls is a no-op here.

func (a *App) addVolumeSlider() {}

func (a *App) wireTrayScroll() {}

func setTraySliderVolume(int, bool) {}
