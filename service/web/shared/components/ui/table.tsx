"use client"

import { useEffect, useRef, useState, type ComponentProps, type RefObject } from "react"
import { cn } from "cn"

/** Whether the element is wider than its box, so it scrolls sideways: such a region takes the keyboard focus (tabIndex 0) and so can be scrolled without a pointer (axe scrollable-region-focusable); one that fits stays out of the tab order. */
export function useOverflows<E extends HTMLElement>(): [RefObject<E | null>, boolean] {
  const ref = useRef<E>(null)
  const [overflows, setOverflows] = useState(false)
  useEffect(() => {
    const el = ref.current
    const check = () => { if (el) setOverflows(el.scrollWidth > el.clientWidth + 1) }
    const observer = new ResizeObserver(check)
    if (el) {
      check()
      observer.observe(el)
      for (const child of el.children) observer.observe(child)
    }
    return () => { observer.disconnect() }
  }, [])
  return [ref, overflows]
}

function Table({ className, ...props }: ComponentProps<"table">) {
  const [ref, overflows] = useOverflows<HTMLDivElement>()
  return (
    <div
      ref={ref}
      data-slot="table-container"
      className="relative w-full overflow-x-auto"
      // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- a box that scrolls sideways must take the focus so the keyboard can scroll it (WCAG 2.1.1, axe scrollable-region-focusable); it does only while it overflows
      tabIndex={overflows ? 0 : undefined}
    >
      <table
        data-slot="table"
        className={cn("w-full caption-bottom text-sm", className)}
        {...props}
      />
    </div>
  )
}

function TableHeader({ className, ...props }: ComponentProps<"thead">) {
  return (
    <thead
      data-slot="table-header"
      className={cn("[&_tr]:border-b", className)}
      {...props}
    />
  )
}

function TableBody({ className, ...props }: ComponentProps<"tbody">) {
  return (
    <tbody
      data-slot="table-body"
      className={cn("[&_tr:last-child]:border-0", className)}
      {...props}
    />
  )
}

function TableFooter({ className, ...props }: ComponentProps<"tfoot">) {
  return (
    <tfoot
      data-slot="table-footer"
      className={cn(
        "border-t bg-muted/50 font-medium [&>tr]:last:border-b-0",
        className
      )}
      {...props}
    />
  )
}

function TableRow({ className, ...props }: ComponentProps<"tr">) {
  return (
    <tr
      data-slot="table-row"
      className={cn(
        "border-b transition-colors hover:bg-muted/50 has-aria-expanded:bg-muted/50 data-[state=selected]:bg-muted",
        className
      )}
      {...props}
    />
  )
}

function TableHead({ className, ...props }: ComponentProps<"th">) {
  return (
    <th
      data-slot="table-head"
      className={cn(
        "h-10 px-2 text-left align-middle font-medium whitespace-nowrap text-foreground [&:has([role=checkbox])]:pr-0",
        className
      )}
      {...props}
    />
  )
}

function TableCell({ className, ...props }: ComponentProps<"td">) {
  return (
    <td
      data-slot="table-cell"
      className={cn(
        "p-2 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0",
        className
      )}
      {...props}
    />
  )
}

function TableCaption({
  className,
  ...props
}: ComponentProps<"caption">) {
  return (
    <caption
      data-slot="table-caption"
      className={cn("mt-4 text-sm text-muted-foreground", className)}
      {...props}
    />
  )
}

export {
  Table,
  TableHeader,
  TableBody,
  TableFooter,
  TableHead,
  TableRow,
  TableCell,
  TableCaption,
}
