set(pf_srcs
    pf.cpp)

add_executable(fwb_pf ${pf_srcs})

IF (UseQt6)
    target_link_libraries(fwb_pf common fwbpf compilerdriver fwcompiler fwbuilder LibXml2::LibXml2 LibXslt::LibXslt ZLIB::ZLIB Qt6::Core Qt6::Widgets)
ELSE()
    target_link_libraries(fwb_pf common fwbpf compilerdriver fwcompiler fwbuilder LibXml2::LibXml2 LibXslt::LibXslt ZLIB::ZLIB Qt5::Core Qt5::Widgets)
ENDIF()

IF (WIN32)
  target_link_libraries(fwb_pf LibLZMA::LibLZMA Iconv::Iconv ws2_32)
ENDIF()

IF (UNIX)
  target_link_libraries(fwb_pf c ${CMAKE_DL_LIBS} util)
ENDIF()

target_compile_options(fwb_pf PRIVATE ${CXX_DEFAULT_FLAGS})

set_source_files_properties(pf.cpp PROPERTIES COMPILE_DEFINITIONS PREFIX=\"${CMAKE_INSTALL_PREFIX}\")

install(TARGETS fwb_pf RUNTIME DESTINATION ${FWB_INSTALL_BINDIR})

