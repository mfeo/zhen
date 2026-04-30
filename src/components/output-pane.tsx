import type { Direction } from "../config.js"

interface OutputPaneProps {
  width: number
  height: number
  direction: Direction
  text: string
  isTranslating: boolean
}

export function OutputPane({ width, height, direction, text, isTranslating }: OutputPaneProps) {
  const displayText = isTranslating && !text ? "Translating..." : text || "Translation will appear here"
  const textColor = !text && !isTranslating ? "#555555" : "#ffffff"
  const title = direction === "zh2en" ? " English Translation " : " 中文翻譯 "

  return (
    <box
      width={width}
      height={height}
      border
      borderStyle="rounded"
      title={title}
      titleAlignment="center"
      flexDirection="column"
    >
      <scrollbox flexGrow={1} stickyScroll>
        <text content={displayText} fg={textColor} wrapMode="word" />
      </scrollbox>
    </box>
  )
}
