import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '../../lib/utils';

const badgeVariants = cva(
  'inline-flex items-center justify-center gap-1.5 rounded border px-2 py-0.5 text-[10px] font-semibold tracking-wider uppercase w-fit whitespace-nowrap shrink-0 transition-colors',
  {
    variants: {
      variant: {
        default: 'border-brand/40 bg-brand-soft text-brand',
        secondary: 'border-transparent bg-secondary text-secondary-foreground',
        outline: 'border-line text-mute',
        // Severity is never carried by colour alone anywhere it matters, but
        // the tint still has to survive peripheral vision on a wall display,
        // so each status gets a filled ground rather than coloured text.
        ok: 'border-ok/30 bg-ok-soft text-ok',
        warn: 'border-warn/30 bg-warn-soft text-warn',
        crit: 'border-crit/30 bg-crit-soft text-crit',
      },
    },
    defaultVariants: { variant: 'default' },
  },
);

function Badge({
  className,
  variant,
  asChild = false,
  ...props
}: React.ComponentProps<'span'> & VariantProps<typeof badgeVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot : 'span';
  return <Comp className={cn(badgeVariants({ variant }), className)} {...props} />;
}

export { Badge, badgeVariants };
