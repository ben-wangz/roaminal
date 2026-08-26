import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

type Props = {
  value: string;
};

// Keep Markdown rendering in the React tree. GFM supplies tables, task lists,
// strikethrough, and autolink literals without enabling raw HTML execution.
export function MarkdownPreview({ value }: Props) {
  return (
    <article className="filesystem-markdown-viewer">
      <Markdown remarkPlugins={[remarkGfm]}>{value}</Markdown>
    </article>
  );
}
