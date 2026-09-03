import * as React from 'react';

import { cn } from '../../lib/utils';

/**
 * Native <label>: it already associates with its control through htmlFor, so
 * no Radix wrapper is needed and no ARIA has to stand in for it.
 */
function Label({ className, ...props }: React.ComponentProps<'label'>) {
  return (
    <label
      className={cn(
        'text-sm leading-none font-medium select-none peer-disabled:cursor-not-allowed peer-disabled:opacity-60',
        className,
      )}
      {...props}
    />
  );
}

export { Label };
