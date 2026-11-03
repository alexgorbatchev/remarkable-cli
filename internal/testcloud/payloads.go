package testcloud

func fixturePayloads(opts Options, f Fixtures) map[string][]byte {
	data := make(map[string][]byte)
	data["folder-hash"] = []byte("folder-meta-hash:folder-1.metadata:0:50\n")
	data["folder-meta-hash"] = []byte(`{"visibleName":"My Folder","type":"CollectionType"}`)

	if opts.Preset == PresetCLI {
		data["doc-hash"] = []byte("meta-hash:doc-1.metadata:0:50\ncontent-hash:doc-1.content:0:100\npdf-hash:doc-1.pdf:0:1000\nstroke-hash:doc-1/page-1.rm:0:200\n")
	} else {
		data["doc-hash"] = []byte("meta-hash:doc-1.metadata:0:50\ncontent-hash:doc-1.content:0:100\npdf-hash:doc-1.pdf:0:1000\nstroke-hash:doc-1/page-uuid-1.rm:0:200\n")
	}
	if opts.Preset == PresetCLI {
		data["meta-hash"] = []byte(`{"visibleName":"My Document","type":"DocumentType","lastModified":"2026-09-29T10:00:00Z"}`)
	} else {
		data["meta-hash"] = []byte(`{"visibleName":"My Document","type":"DocumentType","lastModified":"2026-09-29T10:00:00Z","parent":"folder-1"}`)
	}
	if opts.Preset == PresetCLI {
		data["content-hash"] = []byte(`{"fileType":"pdf","pageCount":1,"pages":["page-1"]}`)
	} else {
		data["content-hash"] = []byte(`{"fileType":"pdf","pageCount":1,"pages":["page-uuid-1"]}`)
	}
	data["pdf-hash"] = f.PDF
	data["stroke-hash"] = f.Strokes

	data["nopdf-hash"] = []byte("nopdf-meta:doc-nopdf.metadata:0:50\nnopdf-content:doc-nopdf.content:0:50\n")
	data["nopdf-meta"] = []byte(`{"visibleName":"Empty Doc","type":"DocumentType"}`)
	data["nopdf-content"] = []byte(`{"fileType":"notebook","pageCount":1,"cPages":{"pages":[{"id":"p1"}]}}`)

	data["nostroke-hash"] = []byte("nostroke-meta:doc-nostroke.metadata:0:50\nnostroke-content:doc-nostroke.content:0:50\npdf-hash:doc-nostroke.pdf:0:1000\n")
	data["nostroke-meta"] = []byte(`{"visibleName":"No Stroke Doc","type":"DocumentType"}`)
	data["nostroke-content"] = []byte(`{"fileType":"pdf","pageCount":1,"pages":["page-nostroke-1"]}`)

	data["suffix-hash"] = []byte("suffix-meta:doc-suffix.metadata:0:50\nsuffix-content:doc-suffix.content:0:50\npdf-hash:other_name.pdf:0:1000\nstroke-hash:random_page.rm:0:200\n")
	data["suffix-meta"] = []byte(`{"visibleName":"Suffix Doc","type":"DocumentType"}`)
	data["suffix-content"] = []byte(`{"fileType":"pdf","pageCount":1,"pages":["random_page"]}`)

	if opts.Preset == PresetCLI {
		data["nocontent-hash"] = []byte("nocontent-meta:nocontent-1.metadata:0:50\n")
	} else {
		data["nocontent-hash"] = []byte("nocontent-meta:doc-nocontent.metadata:0:50\npdf-hash:doc-nocontent.pdf:0:1000\n")
	}
	if opts.Preset == PresetCLI {
		data["nocontent-meta"] = []byte(`{"visibleName":"No Content Document","type":"DocumentType"}`)
	} else {
		data["nocontent-meta"] = []byte(`{"visibleName":"No Content Doc","type":"DocumentType"}`)
	}

	data["corrupt-hash"] = []byte("corrupt-meta:doc-corrupt.metadata:0:50\ncorrupt-pdf:doc-corrupt.pdf:0:50\n")
	data["corrupt-meta"] = []byte(`{"visibleName":"Corrupt Doc","type":"DocumentType"}`)
	data["corrupt-pdf"] = []byte("not-a-valid-pdf")

	if opts.Preset == PresetCLI {
		data["unloadable-hash"] = []byte("unloadable-meta:unloadable-1.metadata:0:50\nunloadable-content:unloadable-1.content:0:50\nunloadable-pdf:unloadable-1.pdf:0:500\n")
	} else {
		data["unloadable-hash"] = []byte("unloadable-meta:doc-unloadable.metadata:0:50\nunloadable-content:doc-unloadable.content:0:50\nunloadable-pdf:doc-unloadable.pdf:0:500\n")
	}
	if opts.Preset == PresetCLI {
		data["unloadable-meta"] = []byte(`{"visibleName":"Unloadable Page PDF","type":"DocumentType"}`)
	} else {
		data["unloadable-meta"] = []byte(`{"visibleName":"Unloadable Page Doc","type":"DocumentType"}`)
	}
	data["unloadable-content"] = []byte(`{"fileType":"pdf","pageCount":2,"pages":["unloadable-page-1","unloadable-page-2"]}`)
	data["unloadable-pdf"] = f.UnloadablePDF

	data["blank-hash"] = []byte("blank-meta:blank-1.metadata:0:50\nblank-content:blank-1.content:0:50\nblank-pdf:blank-1.pdf:0:1000\n")
	data["blank-meta"] = []byte(`{"visibleName":"Unlinked PDF","type":"DocumentType"}`)
	data["blank-content"] = []byte(`{"fileType":"pdf","pageCount":1,"pages":["blank-page-1"]}`)
	data["blank-pdf"] = f.BlankPDF

	data["notebook-hash"] = []byte("notebook-meta:notebook-1.metadata:0:50\nnotebook-content:notebook-1.content:0:50\n")
	data["notebook-meta"] = []byte(`{"visibleName":"Handwritten Notebook","type":"DocumentType"}`)
	data["notebook-content"] = []byte(`{"fileType":"notebook","pageCount":1,"cPages":{"pages":[{"id":"notebook-page-1"}]}}`)

	data["urilinks-hash"] = []byte("urilinks-meta:urilinks-1.metadata:0:50\nurilinks-content:urilinks-1.content:0:50\nurilinks-pdf:urilinks-1.pdf:0:1100\n")
	data["urilinks-meta"] = []byte(`{"visibleName":"URI Links PDF","type":"DocumentType"}`)
	data["urilinks-content"] = []byte(`{"fileType":"pdf","pageCount":2,"pages":["urilinks-page-1","urilinks-page-2"]}`)

	return data
}
