import { OrderDetail } from "@/components/OrderDetail";

export default async function OrderPage({ params }: { params: Promise<{ id: string }> }) {
  return <OrderDetail id={(await params).id} />;
}
