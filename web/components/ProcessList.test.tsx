import assert from 'node:assert/strict';
import test from 'node:test';
import { renderToStaticMarkup } from 'react-dom/server';

import ProcessList from './ProcessList.js';

test('ProcessList renders an empty state when no processes are available', () => {
  const html = renderToStaticMarkup(<ProcessList processes={[]} />);

  assert.match(html, /No process data available/);
});
