package dizee

import (
	"bytes"
	"encoding/xml"
	"fmt"
)

type junitDocument struct {
	XMLName xml.Name
	Attrs   []xml.Attr   `xml:",any,attr"`
	Suites  []junitSuite `xml:"testsuite"`
	Cases   []junitCase  `xml:"testcase"`
}

type junitSuite struct {
	XMLName xml.Name
	Attrs   []xml.Attr  `xml:",any,attr"`
	Cases   []junitCase `xml:"testcase"`
}

type junitCase struct {
	XMLName xml.Name
	Attrs   []xml.Attr    `xml:",any,attr"`
	Failure []junitDetail `xml:"failure"`
	Error   []junitDetail `xml:"error"`
	Skipped *junitDetail  `xml:"skipped"`
}

type junitDetail struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Text    string     `xml:",chardata"`
}

func isJUnit(b []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := decoder.Token()
		if err != nil {
			return false
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start.Name.Local == "testsuite" || start.Name.Local == "testsuites"
		}
	}
}

func shrinkJUnit(b []byte) ([]byte, error) {
	var doc junitDocument
	if err := xml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	if err := encodeJUnitDocument(encoder, doc); err != nil {
		return nil, err
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	if bytes.HasSuffix(b, []byte("\n")) {
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func encodeJUnitDocument(encoder *xml.Encoder, doc junitDocument) error {
	start := xml.StartElement{Name: doc.XMLName, Attr: doc.Attrs}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	for _, suite := range doc.Suites {
		if err := encodeJUnitSuite(encoder, suite); err != nil {
			return err
		}
	}
	if err := encodeJUnitCases(encoder, doc.Cases); err != nil {
		return err
	}
	return encoder.EncodeToken(start.End())
}

func encodeJUnitSuite(encoder *xml.Encoder, suite junitSuite) error {
	start := xml.StartElement{Name: suite.XMLName, Attr: suite.Attrs}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	if err := encodeJUnitCases(encoder, suite.Cases); err != nil {
		return err
	}
	return encoder.EncodeToken(start.End())
}

func encodeJUnitCases(encoder *xml.Encoder, cases []junitCase) error {
	passed := 0
	flushPassed := func() error {
		if passed == 0 {
			return nil
		}
		err := encoder.EncodeToken(xml.Comment(fmt.Sprintf("[x%d passed]", passed)))
		passed = 0
		return err
	}
	for _, testCase := range cases {
		if len(testCase.Failure) == 0 && len(testCase.Error) == 0 && testCase.Skipped == nil {
			passed++
			continue
		}
		if err := flushPassed(); err != nil {
			return err
		}
		if err := encodeJUnitCase(encoder, testCase); err != nil {
			return err
		}
	}
	return flushPassed()
}

func encodeJUnitCase(encoder *xml.Encoder, testCase junitCase) error {
	start := xml.StartElement{Name: testCase.XMLName, Attr: testCase.Attrs}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	for _, details := range [][]junitDetail{testCase.Failure, testCase.Error} {
		for _, detail := range details {
			if err := encodeJUnitDetail(encoder, detail); err != nil {
				return err
			}
		}
	}
	if testCase.Skipped != nil {
		if err := encodeJUnitDetail(encoder, *testCase.Skipped); err != nil {
			return err
		}
	}
	return encoder.EncodeToken(start.End())
}

func encodeJUnitDetail(encoder *xml.Encoder, detail junitDetail) error {
	start := xml.StartElement{Name: detail.XMLName, Attr: detail.Attrs}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	if err := encoder.EncodeToken(xml.CharData(detail.Text)); err != nil {
		return err
	}
	return encoder.EncodeToken(start.End())
}
