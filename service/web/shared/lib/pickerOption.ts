/** One choice of a search select that reads its options on the server: the main text, the details that tell two of the
same name apart, the version and unit of the record (a bind needs them) and, for a record that cannot be chosen, why not. */
export interface PickerOption { id: string; label: string; detail: string; version: number; unitId: string | null; blocked: string | null }
