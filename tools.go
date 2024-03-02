package httpServer

/*
 * (C) Copyright 2024 Johan Michel PIQUET, France (https://johanpiquet.fr/).
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import (
	"io"
	"os"
	"path"
)

func MethodNameToMethodCode(method string) HttpMethod {
	switch method {
	case "GET":
		return HttpMethodGET
	case "POST":
		return HttpMethodPOST
	case "HEAD":
		return HttpMethodHEAD
	case "DELETE":
		return HttpMethodDELETE
	case "PUT":
		return HttpMethodPUT
	case "CONNECT":
		return HttpMethodCONNECT
	case "OPTIONS":
		return HttpMethodOPTIONS
	case "TRACE":
		return HttpMethodTRACE
	case "PATCH":
		return HttpMethodPATCH
	default:
		return HttpMethodGET
	}
}

func SaveStreamBodyToFile(reader io.Reader, outFilePath string) error {
	fo, err := os.Create(outFilePath)
	if err != nil {
		err = os.MkdirAll(path.Dir(outFilePath), os.ModePerm)

		if err != nil {
			return err
		}
	}

	defer fo.Close()

	buf := make([]byte, 1024)
	for {
		n, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}

		if n == 0 {
			break
		}

		if _, err := fo.Write(buf[:n]); err != nil {
			return err
		}
	}

	return nil
}

func spaceRight(spaces int, text string) string {
	diff := spaces - len(text)

	for i := 0; i < diff; i++ {
		text += " "
	}

	return text
}
