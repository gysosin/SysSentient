import * as React from 'react';

import { cn } from '../../lib/utils';

/**
 * The heading pattern every screen and panel in the console is built from: a
 * small, wide-tracked eyebrow naming the *kind* of information, then the title
 * in the display face.
 *
 * The eyebrow is doing real work, not decoration. An operations console is a
 * wall of numbers, and two words of category above a heading is what lets
 * someone scanning from across the room find the region they want before they
 * start reading. It also gives every panel the same vertical rhythm, which is
 * why the layout holds together at eight panels per screen.
 */
export function SectionHeading({
  eyebrow,
  title,
  action,
  className,
  id,
}: {
  eyebrow?: string;
  title: string;
  action?: React.ReactNode;
  className?: string;
  id?: string;
}) {
  return (
    <div className={cn('flex min-w-0 items-center justify-between gap-4', className)}>
      <div className="min-w-0">
        {eyebrow && (
          <p className="text-mute text-[10px] font-medium tracking-[0.2em] uppercase">{eyebrow}</p>
        )}
        <h2
          id={id}
          className="text-foreground font-display mt-1 truncate text-lg font-semibold tracking-[0.08em] uppercase"
        >
          {title}
        </h2>
      </div>
      {action}
    </div>
  );
}

/**
 * The page-level version: same idea, one size up, with a sentence of context.
 *
 * The description is not filler — each screen states what question it answers,
 * so someone landing on it mid-incident knows within one line whether they are
 * in the right place.
 */
export function ScreenHeading({
  eyebrow,
  title,
  description,
  action,
}: {
  eyebrow: string;
  title: string;
  description: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="mb-7 flex flex-wrap items-end justify-between gap-4">
      <div className="max-w-2xl">
        <p className="text-brand text-[10px] font-semibold tracking-[0.22em] uppercase">{eyebrow}</p>
        <h1 className="text-foreground font-display mt-2 text-4xl font-bold tracking-tight uppercase sm:text-5xl">
          {title}
        </h1>
        <p className="text-muted-foreground mt-3 text-sm leading-relaxed">{description}</p>
      </div>
      {action}
    </div>
  );
}

/**
 * A hairline-divided cell grid. `gap-px` over a `bg-line` ground turns the gaps
 * themselves into the rules, so a KPI strip reads as one instrument panel
 * rather than four detached cards.
 */
export function HairlineGrid({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      className={cn('bg-line grid gap-px overflow-hidden rounded-xl border-0', className)}
      {...props}
    />
  );
}
