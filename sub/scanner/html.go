package scanner

import (
	"fmt"
	"github.com/Symantec/Dominator/lib/format"
	"io"
	"time"
)

func (fsh *FileSystemHistory) writeHtml(writer io.Writer) {
	fmt.Fprintf(writer, "Scan count: %d<br>\n", fsh.scanCount)
	fmt.Fprintf(writer, "Generation count: %d<br>\n", fsh.generationCount)
	if fsh.scanCount > 0 {
		fmt.Fprintf(writer, "Last scan completed: %s<br>\n", fsh.timeOfLastScan)
		fmt.Fprintf(writer, "Duration of last scan: %s<br>\n",
			fsh.durationOfLastScan)
		fsh.fileSystem.WriteHtml(writer)
		tmp := format.FormatBytes(uint64(float64(
			fsh.fileSystem.TotalDataBytes) / fsh.durationOfLastScan.Seconds()))
		fmt.Fprintf(writer, "Scan rate: %s/s<br>\n", tmp)
	}
	fmt.Fprintf(writer, "Duration of current scan: %s<br>\n",
		time.Since(fsh.timeOfLastScan))
	if fsh.generationCount > 0 {
		fmt.Fprintf(writer, "Last change: %s<br>\n", fsh.timeOfLastChange)
	}
}

func (fs *FileSystem) writeHtml(writer io.Writer) {
	fmt.Fprintf(writer, "Scanned: %s<br>\n",
		format.FormatBytes(fs.TotalDataBytes))
}

func (configuration *Configuration) writeHtml(writer io.Writer) {
	speed := "unknown"
	ctx := configuration.NetworkReaderContext
	if ctx.MaximumSpeed() > 0 {
		speed = fmt.Sprintf("%s/s (%d%% of %s/s)",
			format.FormatBytes(
				ctx.MaximumSpeed()*uint64(ctx.SpeedPercent())/100),
			ctx.SpeedPercent(), format.FormatBytes(ctx.MaximumSpeed()))
	}
	fmt.Fprintf(writer, "Network Speed: %s<br>\n", speed)
}
