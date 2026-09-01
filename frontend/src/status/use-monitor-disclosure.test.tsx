import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useMonitorDisclosure } from './use-monitor-disclosure';

const environment = vi.hoisted(() => ({ mobileMode: true }));
vi.mock('../input/mobile-mode', () => ({ useMobileMode: () => environment.mobileMode }));

function Harness({ resetKey }: { resetKey: string | null }) {
  const { expanded, setExpanded } = useMonitorDisclosure(resetKey);
  return <button type="button" data-expanded={expanded} onClick={() => setExpanded(true)} />;
}

describe('useMonitorDisclosure', () => {
  let renderer: ReactTestRenderer | null = null;

  beforeEach(async () => {
    environment.mobileMode = true;
    if (renderer) await act(async () => renderer?.unmount());
    renderer = null;
  });

  it('collapses an expanded mobile monitor when the reset key changes', async () => {
    await act(async () => {
      renderer = create(<Harness resetKey="instance-a" />);
    });
    expect(renderer?.root.findByType('button').props['data-expanded']).toBe(false);

    await act(async () => {
      renderer?.root.findByType('button').props.onClick();
    });
    expect(renderer?.root.findByType('button').props['data-expanded']).toBe(true);

    await act(async () => {
      renderer?.update(<Harness resetKey="instance-b" />);
    });
    expect(renderer?.root.findByType('button').props['data-expanded']).toBe(false);
  });
});
