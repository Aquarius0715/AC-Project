import { notFound } from "next/navigation";
// Any address under this area that no route matches: the area's not-found renders it inside the app layout (UI/UX loop R54).
export default function Missing(): never {
  notFound();
}
