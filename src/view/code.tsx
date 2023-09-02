import * as React from "react";
import { RendererContext, RendererOpts, LineStateType } from "../types";
import Editor from "@monaco-editor/react";
import { GlobalModel } from "../model";

class SourceCodeRenderer extends React.Component<
    {
        data: Blob;
        cmdstr: string;
        cwd: string;
        readOnly: boolean;
        exitcode: number;
        context: RendererContext;
        opts: RendererOpts;
        savedHeight: number;
        scrollToBringIntoViewport: () => void;
        lineState: LineStateType;
    },
    {}
> {
    /**
     * codeCache is a Hashmap with key=screenId:lineId:filepath and value=code
     * Editor should never read the code directly from the filesystem. it should read from the cache.
     * Upon loading a file (props.data contains the file-contents) FOR THE FIRST TIME,
     * we will put it in the cache, and will update the contents of the cache upon every onChange().
     * ALl this is to ensure that the file contents doesnt get reloaded when the line scrolls out of the viewport
     * (and hence the react component gets destroyed)
     */
    static codeCache = new Map();

    filePath;
    cacheKey;
    originalData;
    constructor(props) {
        super(props);
        this.editorRef = React.createRef();
        this.state = {
            code: "",
            language: "",
            languages: [],
            selectedLanguage: "",
            isFullWindow: false,
            isSave: false,
            editorHeight: props.savedHeight,
            errorMessage: null,
        };
    }

    componentDidMount(): void {
        this.filePath = this.props.lineState["prompt:file"];
        const { screenId, lineId } = this.props.context;
        this.cacheKey = `${screenId}-${lineId}-${this.filePath}`;
        const code = SourceCodeRenderer.codeCache.get(this.cacheKey);
        if (code) {
            this.setState({ code });
        } else
            this.props.data.text().then((code) => {
                this.originalData = code;
                this.setState({ code });
                SourceCodeRenderer.codeCache.set(this.cacheKey, code);
            });
    }

    handleEditorDidMount = (editor, monaco) => {
        const extension =
            (this.filePath && this.filePath.match(/(?:[^\\\/:*?"<>|\r\n]+\.)([a-zA-Z0-9]+)\b/)?.[1]) || "";
        const detectedLanguage = monaco.languages
            .getLanguages()
            .find((lang) => lang.extensions?.includes("." + extension));
        const languages = monaco.languages.getLanguages().map((lang) => lang.id);
        this.setState({ languages });
        if (detectedLanguage) {
            this.editorRef.current = editor;
            const model = editor.getModel();
            if (model) {
                monaco.editor.setModelLanguage(model, detectedLanguage.id);
                this.setState({ selectedLanguage: detectedLanguage.id, language: detectedLanguage.id });
            }
        }
        this.setEditorHeight();
    };

    handleLanguageChange = (event) => {
        const selectedLanguage = event.target.value;
        this.setState({ selectedLanguage });
        if (this.editorRef.current) {
            const model = this.editorRef.current.getModel();
            if (model) {
                monaco.editor.setModelLanguage(model, selectedLanguage);
                this.setState({ language: selectedLanguage });
            }
        }
    };

    toggleFit = () => {
        const isFullWindow = !this.state.isFullWindow;
        this.setState({ isFullWindow });
        this.setEditorHeight();
        setTimeout(() => this.props.scrollToBringIntoViewport(), 350);
    };

    doSave = () => {
        const { screenId, lineId } = this.props.context;
        const encodedCode = new TextEncoder().encode(this.state.code);
        debugger;
        GlobalModel.writeRemoteFile(screenId, lineId, this.filePath, encodedCode, { useTemp: true })
            .then(() => {
                this.originalData = this.state.code;
                this.setState({ isSave: false });
            })
            .catch(() => {
                this.setState({ errorMessage: "File could not be saved" });
                setTimeout(() => this.setState({ errorMessage: null }), 3000);
            });
    };

    handleEditorChange = (code) => {
        this.setState({ isFullWindow: true, code });
        SourceCodeRenderer.codeCache.set(this.cacheKey, code);
        this.setEditorHeight();
        setTimeout(() => this.props.scrollToBringIntoViewport(), 350);
        this.props.data.text().then((originalCode) => this.setState({ isSave: code !== originalCode }));
    };

    setEditorHeight = () => {
        const fullWindowHeight = parseInt(this.props.opts.maxSize.height);
        let _editorHeight = fullWindowHeight;
        if (!this.state.isFullWindow) {
            const noOfLines = this.state.code.split("\n").length;
            _editorHeight = Math.min(noOfLines * GlobalModel.termFontSize.get() * 1.5 + 10, fullWindowHeight);
        }
        this.setState({ editorHeight: _editorHeight });
    };

    render() {
        const { opts, exitcode } = this.props;
        const { lang, code, isSave } = this.state;

        if (!code)
            return <div className="renderer-container code-renderer" style={{ height: this.props.savedHeight }} />;

        if (exitcode === 1)
            return (
                <div
                    className="renderer-container code-renderer"
                    style={{
                        fontSize: GlobalModel.termFontSize.get(),
                        fontFamily: "JetBrains Mono",
                        color: "white",
                    }}
                >
                    {code}
                </div>
            );

        return (
            <div className="renderer-container code-renderer">
                <div className="scroller" style={{ maxHeight: opts.maxSize.height, paddingBottom: "15px" }}>
                    <Editor
                        theme="hc-black"
                        height={this.state.editorHeight}
                        defaultLanguage={lang}
                        defaultValue={code}
                        onMount={this.handleEditorDidMount}
                        options={{
                            scrollBeyondLastLine: false,
                            fontSize: GlobalModel.termFontSize.get(),
                            fontFamily: "JetBrains Mono",
                            readOnly: this.props.readOnly,
                        }}
                        onChange={this.handleEditorChange}
                    />
                </div>
                <div style={{ position: "absolute", bottom: "-3px", right: 0 }}>
                    <select
                        className="dropdown"
                        value={this.state.selectedLanguage}
                        onChange={this.handleLanguageChange}
                        style={{ minWidth: "6rem", maxWidth: "6rem", marginRight: "8px" }}
                    >
                        {this.state.languages.map((lang, index) => (
                            <option key={index} value={lang}>
                                {lang}
                            </option>
                        ))}
                    </select>
                    <div className="cmd-hints" style={{ minWidth: "6rem", maxWidth: "6rem" }}>
                        <div onClick={this.toggleFit} className="hint-item color-white">
                            {this.state.isFullWindow ? `shrink` : `expand`}
                        </div>
                    </div>
                    {!this.props.opts.readOnly && (
                        <div className="cmd-hints" style={{ minWidth: "6rem", maxWidth: "6rem", marginLeft: "-18px" }}>
                            <div
                                onClick={this.doSave}
                                className={`hint-item ${isSave ? "save-enabled" : "save-disabled"}`}
                            >
                                {"save"}
                            </div>
                        </div>
                    )}
                </div>
                {this.state.errorMessage && (
                    <div style={{ position: "absolute", bottom: "-3px", left: "14px" }}>
                        <div
                            className="error"
                            style={{ fontSize: GlobalModel.termFontSize.get(), fontFamily: "JetBrains Mono" }}
                        >
                            {this.state.errorMessage}
                        </div>
                    </div>
                )}
            </div>
        );
    }
}

export { SourceCodeRenderer };
