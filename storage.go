package websocketnats

import (
	"sync"
)

//const NOT_LOGGED_LIFE_TIME = 5 * time.Second
//const PING_TIMEOUT = 10 * time.Minute

// ConnectionsStats connection status
type ConnectionsStats struct {
	NumberOfUsers                int
	NumberOfDevices              int
	NumberOfNotLoggedConnections int
}

// ConnectionsStorage connection storage (pool)
type ConnectionsStorage struct {
	mutex                        sync.RWMutex
	connectionsByID              map[ConnectionID]*Connection
	connectionsByUserID          map[UserID]map[DeviceID]*Connection
	connectionsByDeviceID        map[DeviceID]*Connection // one connection per device
	numberOfNotLoggedConnections int
}

// NewConnectionsStorage init connections storage
func NewConnectionsStorage() *ConnectionsStorage {
	return &ConnectionsStorage{
		mutex:                        sync.RWMutex{},
		connectionsByID:              make(map[ConnectionID]*Connection),
		connectionsByUserID:          make(map[UserID]map[DeviceID]*Connection),
		connectionsByDeviceID:        make(map[DeviceID]*Connection),
		numberOfNotLoggedConnections: 0,
	}
}

// AddNewConnection add new connection to storage
func (s *ConnectionsStorage) AddNewConnection(connection *Connection) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.numberOfNotLoggedConnections++
	s.connectionsByID[connection.id] = connection
}

// OnLogin onlogin hook to check if the connection exists in connection pool
func (s *ConnectionsStorage) OnLogin(connection *Connection) *Connection {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, userID, deviceID := connection.GetInfo()
	if userID == "" {
		return nil
	}

	s.numberOfNotLoggedConnections--

	deviceConnectionBefore := s.connectionsByDeviceID[connection.deviceID]
	if deviceConnectionBefore != nil {
		s.removeConnection(deviceConnectionBefore)
	}
	s.connectionsByDeviceID[deviceID] = connection

	userConnections := s.connectionsByUserID[userID]
	if userConnections == nil {
		userConnections = make(map[DeviceID]*Connection)
		s.connectionsByUserID[userID] = userConnections
	}
	userConnections[deviceID] = connection

	return deviceConnectionBefore
}

// RemoveConnection remove connnection from pool
func (s *ConnectionsStorage) RemoveConnection(connection *Connection) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.removeConnection(connection)
}

func (s *ConnectionsStorage) removeConnection(connection *Connection) {
	connectionID, userID, deviceID := connection.GetInfo()

	connectionBefore := s.connectionsByID[connectionID]
	if connectionBefore == nil {
		return
	}

	delete(s.connectionsByID, connectionID)

	if userID == "" {
		s.numberOfNotLoggedConnections--
		return
	}

	userConnections := s.connectionsByUserID[userID]
	if userConnections != nil {
		delete(userConnections, deviceID)
		if len(userConnections) == 0 {
			delete(s.connectionsByUserID, userID)
		}
	}

	deviceConnection := s.connectionsByDeviceID[deviceID]
	if deviceConnection != nil {
		delete(s.connectionsByDeviceID, deviceID)
	}
}

// GetUserConnections get connections by userID
func (s *ConnectionsStorage) GetUserConnections(userID UserID) map[DeviceID]*Connection {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.connectionsByUserID[userID]
}

// GetDeviceConnection get connections by device ID
func (s *ConnectionsStorage) GetDeviceConnection(deviceID DeviceID) *Connection {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.connectionsByDeviceID[deviceID]
}

// GetConnectionByID get connection by ID
func (s *ConnectionsStorage) GetConnectionByID(connectionID ConnectionID) *Connection {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.connectionsByID[connectionID]
}

// GetStats get connection storage status
func (s *ConnectionsStorage) GetStats() ConnectionsStats {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	stats := ConnectionsStats{
		NumberOfDevices:              len(s.connectionsByDeviceID),
		NumberOfUsers:                len(s.connectionsByUserID),
		NumberOfNotLoggedConnections: s.numberOfNotLoggedConnections,
	}

	return stats
}

// RemoveIf remove connection wrapped by a condition and callback
func (s *ConnectionsStorage) RemoveIf(condition func(con *Connection) bool, afterRemove func(con *Connection)) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for id, connection := range s.connectionsByID {
		if condition(connection) {
			_, userID, deviceID := connection.GetInfo()

			delete(s.connectionsByID, id)

			if deviceID != "" {
				delete(s.connectionsByDeviceID, deviceID)
			}

			if userID != "" {
				userConnections := s.connectionsByUserID[userID]
				if userConnections != nil {
					if len(userConnections) == 1 {
						delete(s.connectionsByUserID, userID)
					} else {
						delete(userConnections, deviceID)
					}
				}
			}

			afterRemove(connection)
		}
	}
}
