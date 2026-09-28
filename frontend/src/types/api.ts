export type ChargerStatus = "AVAILABLE" | "CHARGING" | "MAINTENANCE";
export type ReservationStatus =
  | "SCHEDULED"
  | "WAITING"
  | "ACTIVE"
  | "COMPLETED"
  | "EXPIRED"
  | "CANCELLED";

export interface User {
  id: string;
  username: string;
}

export interface Charger {
  id: string;
  name: string;
  location: string;
  status: ChargerStatus;
  updated_at: string;
}

export interface Reservation {
  id: string;
  user_id: string;
  charger_id: string;
  start_time: string;
  end_time: string;
  status: ReservationStatus;
  created_at: string;
  updated_at: string;
}

export interface StatusEvent {
  event: "CHARGER_STATUS_UPDATED";
  data: Pick<Charger, "status" | "updated_at"> & { charger_id: string };
}

export interface ReserveInput {
  user_id: string;
  start_time: string;
  end_time: string;
}
