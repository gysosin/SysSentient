import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '../../lib/utils';

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-[12px] font-semibold transition-colors disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg:not([class*='size-'])]:size-3.5 shrink-0 outline-none focus-visible:ring-[3px] focus-visible:ring-ring/40",
  {
    variants: {
      variant: {
        // Coral. The product's own voice: the action this screen is for.
        default: 'bg-primary text-primary-foreground hover:bg-primary/90',
        // Amber, and the only place it appears on a control. Amber means the
        // machine is speaking, so an AI action must never look like an
        // ordinary button and an ordinary button must never borrow it.
        accent: 'bg-accent text-accent-foreground hover:bg-accent/90',
        destructive: 'bg-destructive text-destructive-foreground hover:bg-destructive/90',
        // `accent` used to be shadcn's neutral hover surface; in this palette
        // it is amber, so neutral controls rest on the panel ramp instead.
        outline: 'border border-line bg-panel-strong text-foreground hover:bg-muted',
        secondary: 'bg-secondary text-secondary-foreground hover:bg-secondary/80',
        ghost: 'hover:bg-panel-strong hover:text-foreground',
        link: 'text-brand underline-offset-4 hover:underline',
      },
      size: {
        default: 'h-9 px-3.5 py-2.5',
        sm: 'h-8 rounded-md px-3 text-[11px]',
        lg: 'h-10 rounded-md px-6 text-[13px]',
        icon: 'size-9',
      },
    },
    defaultVariants: { variant: 'default', size: 'default' },
  },
);

function Button({
  className,
  variant,
  size,
  asChild = false,
  ...props
}: React.ComponentProps<'button'> &
  VariantProps<typeof buttonVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot : 'button';
  return <Comp className={cn(buttonVariants({ variant, size, className }))} {...props} />;
}

export { Button, buttonVariants };
