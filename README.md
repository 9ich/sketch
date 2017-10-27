	NAME
		sketch - sketch images

	SYNOPSIS
		sketch [ -p ] [ -i iter ] [ -l len ] [ -s sec ] file ...

	DESCRIPTION
		Sketch uses lines of random length, orientation and
		color to approximate input images. The line colors are
		randomly selected from a palette of all the colors in
		the input image.

		If no arguments are given, sketch will process
		incrementing input filenames, starting with
		input001.png. This number can be changed using the
		-1 option.

	OPTIONS
		-1 num
		      offset of first frame (default 1)
		-P    parallelize (slower on short lines)
		-i iter
		      number of iterations (-1 for infinite) (default
		      5000000)
		-l len
		      line length limit (default 40)
		-n frames
		      number of input frames to sketch
		-p    remove duplicate colors from palette
		-s sec
		      interval between incremental saves, in seconds
		      (default -1)
		-t sec
		      statistics reporting interval, in seconds
		      (default 1)

	EXAMPLES
		ffmpeg -i input.webm input%03d.png
		sketch input*.png
		ffmpeg -i frame%03d.png -c:v vp8 output.webm
