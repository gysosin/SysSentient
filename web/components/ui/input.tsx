import * as React from 'react';

import { cn } from '../../lib/utils';

function Input({ className, type, ...props }: React.ComponentProps<'input'>) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        'border-input bg-background/60 flex h-8 w-full min-w-0 rounded-md border px-3 py-1 text-sm transition-[color,box-shadow] outline-none',
        'placeholder:text-muted-foreground/70 selection:bg-primary selection:text-primary-foreground',
        'focus-visible:border-ring focus-visible:ring-ring/40 focus-visible:ring-[3px]',
        'disabled:cursor-not-allowed disabled:opacity-50',
        className,
      )}
      {...props}
    />
  );
}

export { Input };
