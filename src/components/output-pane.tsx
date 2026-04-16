interface OutputPaneProps {
  width: number
  height: number
  text: string
  isTranslating: boolean
}

export function OutputPane({ width, height, text, isTranslating }: OutputPaneProps) {
  const displayText = isTranslating && !text ? "Translating..." : text || "Translation will appear here"
  const textColor = !text && !isTranslating ? "#555555" : "#ffffff"

  return (
    <box
      width={width}
      height={height}
      border
      borderStyle="rounded"
      title=" English Translation "
      titleAlignment="center"
      flexDirection="column"
    >
      <scrollbox flexGrow={1} stickyScroll>
        <text content={displayText} fg={textColor} wrapMode="word" />
      </scrollbox>
    </box>
  )
}
