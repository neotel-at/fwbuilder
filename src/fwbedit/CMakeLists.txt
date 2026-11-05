find_package(OpenSSL REQUIRED)

set(fwbedit_srcs
    fwbedit.cpp
    new_object.cpp
    repair_tree.cpp
    list_object.cpp
    merge.cpp
    import.cpp
    ../libgui/FWBTree.cpp
    ../libgui/platforms.cpp)

add_executable(fwbedit ${fwbedit_srcs})

IF (UseQt6)
    target_link_libraries(fwbedit gui import fwbparser antlr common iptlib fwbpf fwbcisco compilerdriver fwcompiler fwbuilder LibXml2::LibXml2 LibXslt::LibXslt ZLIB::ZLIB OpenSSL::Crypto Qt6::Core Qt6::Widgets Qt6::PrintSupport)
ELSE()
    target_link_libraries(fwbedit gui import fwbparser antlr common iptlib fwbpf fwbcisco compilerdriver fwcompiler fwbuilder LibXml2::LibXml2 LibXslt::LibXslt ZLIB::ZLIB OpenSSL::Crypto Qt5::Core Qt5::Widgets Qt5::PrintSupport)
ENDIF()

IF (WIN32)
  target_link_libraries(fwbedit LibLZMA::LibLZMA Iconv::Iconv)
ENDIF()

IF (UNIX)
  target_link_libraries(fwbedit c ${CMAKE_DL_LIBS} util)
ENDIF()

IF (NETSNMP_FOUND)
  target_link_libraries(fwbedit netsnmp)
ENDIF()

target_compile_options(fwbedit PRIVATE ${CXX_DEFAULT_FLAGS})

set_source_files_properties(fwbedit.cpp PROPERTIES COMPILE_DEFINITIONS PREFIX=\"${CMAKE_INSTALL_PREFIX}\")

install(TARGETS fwbedit RUNTIME DESTINATION ${FWB_INSTALL_BINDIR})

