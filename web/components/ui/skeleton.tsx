import * as React from 'react';

import { cn } from '../../lib/utils';

/** Shimmering placeholder. Used instead of showing zeros, which read as real
 *  readings — the original dashboard displayed "CPU 0.0%" before any data
 *  arrived, indistinguishable from a genuinely idle machine. */
function Skeleton({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="skeleton"
      className={cn('bg-muted/60 shimmer rounded-md', className)}
      {...props}
    />
  );
}

export { Skeleton };
