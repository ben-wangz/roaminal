import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { MarkdownPreview } from './markdown-preview';

describe('markdown preview', () => {
  it('renders GFM tables and task lists through the Markdown library', () => {
    const html = renderToStaticMarkup(
      <MarkdownPreview value={'| Name | State |\n| --- | --- |\n| alpha | ready |\n\n- [x] shipped\n- [ ] pending'} />,
    );

    expect(html).toContain('<table>');
    expect(html).toContain('<thead>');
    expect(html).toContain('<th>Name</th>');
    expect(html).toContain('<td>ready</td>');
    expect(html).toContain('type="checkbox"');
  });

  it('does not execute raw HTML from a remote document', () => {
    const html = renderToStaticMarkup(<MarkdownPreview value="<script>alert('xss')</script>" />);

    expect(html).not.toContain('<script>');
    expect(html).toContain('&lt;script&gt;');
  });
});
