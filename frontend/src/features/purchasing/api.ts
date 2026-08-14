import api from '../../api/client'; import type { DeliveryCalendar, GoodsReceipt, GoodsReceiptLine, PurchaseOrder, PurchaseOrderLine } from './types';
export const purchasingApi={
 listOrders:(params?:Record<string,string>)=>api.get<{purchaseOrders:PurchaseOrder[]}>('/product/purchase-orders',{params}).then(r=>r.data),
 getOrder:(id:string)=>api.get<{purchaseOrder:PurchaseOrder;lines:PurchaseOrderLine[]}>(`/product/purchase-orders/${id}`).then(r=>r.data),
 createOrder:(input:unknown)=>api.post<{purchaseOrder:PurchaseOrder;lines:PurchaseOrderLine[]}>('/product/purchase-orders',input).then(r=>r.data),
 updateOrder:(id:string,input:unknown)=>api.patch<{purchaseOrder:PurchaseOrder;lines:PurchaseOrderLine[]}>(`/product/purchase-orders/${id}`,input).then(r=>r.data),
 action:(id:string,action:'submit'|'approve'|'cancel',input:{version:number;note?:string})=>api.post<{purchaseOrder:PurchaseOrder}>(`/product/purchase-orders/${id}/${action}`,input).then(r=>r.data),
 calendars:(params?:Record<string,string>)=>api.get<{deliveryCalendars:DeliveryCalendar[]}>('/product/delivery-calendars',{params}).then(r=>r.data),
 createCalendar:(input:unknown)=>api.post<{deliveryCalendar:DeliveryCalendar}>('/product/delivery-calendars',input).then(r=>r.data),
 updateCalendar:(id:string,input:unknown)=>api.patch<{deliveryCalendar:DeliveryCalendar}>(`/product/delivery-calendars/${id}`,input).then(r=>r.data),
 deleteCalendar:(id:string)=>api.delete(`/product/delivery-calendars/${id}`),
 receipts:(params?:Record<string,string>)=>api.get<{goodsReceipts:GoodsReceipt[]}>('/product/goods-receipts',{params}).then(r=>r.data),
 orderReceipts:(id:string)=>api.get<{goodsReceipts:GoodsReceipt[]}>(`/product/purchase-orders/${id}/goods-receipts`).then(r=>r.data),
 receipt:(id:string)=>api.get<{receipt:GoodsReceipt;lines:GoodsReceiptLine[]}>(`/product/goods-receipts/${id}`).then(r=>r.data),
 createReceipt:(orderId:string,input:unknown)=>api.post<{receipt:GoodsReceipt;lines:GoodsReceiptLine[]}>(`/product/purchase-orders/${orderId}/receipts`,input).then(r=>r.data),
 reverseReceipt:(id:string)=>api.post<{receipt:GoodsReceipt;lines:GoodsReceiptLine[]}>(`/product/goods-receipts/${id}/reverse`,{idempotencyKey:crypto.randomUUID()}).then(r=>r.data), document:(id:string)=>api.get(`/product/purchase-orders/${id}/document`,{responseType:'blob'}).then(r=>r.data as Blob), sendDocument:(id:string,input:{recipientEmail:string;idempotencyKey:string})=>api.post<{delivery:{id:string;purchaseOrderId:string;orderVersion:number;status:'sent'|'pending'}}>(`/product/purchase-orders/${id}/document/send`,input,{headers:{'Idempotency-Key':input.idempotencyKey}}).then(r=>r.data),
};
