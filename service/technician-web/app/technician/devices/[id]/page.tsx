import { DevicesView } from "@ac/web/components/TechDevices";
export default async function DeviceById({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <DevicesView initial={id} />;
}
