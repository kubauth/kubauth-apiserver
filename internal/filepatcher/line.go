/*
Copyright 2025 Kubotal

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package filepatcher

import (
	"fmt"
)

func (lop *lineOp) patch(lines []string, remove bool) ([]string, error) {
	if remove {
		return lop.unPatch(lines)
	}
	// If the line is present, just update it, and job is done, so exit
	for idx, line := range lines {
		if lop.regex.Match([]byte(line)) {
			lines[idx] = lop.padding + lop.op.Line
			return lines, nil
		}
	}
	// Line was not found. Will insert it
	newLines := make([]string, 0, len(lines)+20)
	for idx, line := range lines {
		switch lop.state {
		case lineInit:
			//fmt.Printf("will try '%s'\n", line)
			if lop.insertAfter.Match([]byte(line)) {
				newLines = append(newLines, line)
				newLines = append(newLines, lop.padding+lop.op.Line)
				lop.state = lineFound
			} else {
				newLines = append(newLines, line)
			}
		case lineFound:
			newLines = append(newLines, line)
		default:
			return nil, fmt.Errorf("unhandlded state '%d' at line %d on first pass", lop.state, idx)
		}
	}
	if lop.state == lineInit {
		// Was not added. Add at the end
		newLines = append(newLines, lop.padding+lop.op.Line)
		lop.state = lineFound
	}
	return newLines, nil
}

func (lop *lineOp) unPatch(lines []string) ([]string, error) {
	newLines := make([]string, 0, len(lines)+20)
	for _, line := range lines {
		if !lop.regex.Match([]byte(line)) {
			newLines = append(newLines, line)
		}
	}
	return newLines, nil
}
